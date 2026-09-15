// techo5-aec — WebRTC's echo canceller as a small helper the daemon talks to over pipes.
//
// The daemon is static Go with no C in it; WebRTC's AudioProcessing is C++ and Alpine ships it
// (webrtc-audio-processing-1). So this process sits between: it reads 20 ms blocks from stdin —
// 320 samples of microphone, then 320 samples of the playback loopback, int16 little-endian at
// 16 kHz — runs the far end through the canceller's reverse stream and the microphone through the
// forward one, and writes the 320 processed samples to stdout. One block in, one block out, in
// order, so the daemon's read blocks for exactly as long as the processing takes.
//
// The loopback is sample aligned with the microphones (the FPGA delivers them in one frame), so the
// reported stream delay is 0 and the plain filter converges; see docs/porting-plan.md, "Echo
// cancellation, WebRTC grade", for the measurements this follows.
//
//   techo5-aec [--aec low|moderate|high] [--ns off|low|moderate|high|veryhigh] [--gain-db N]
//
// Build: tools/linux/build-aec.sh (in WSL, armv7 Alpine under QEMU).

#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <memory>
#include <string>
#include <unistd.h>

#include <modules/audio_processing/include/audio_processing.h>

namespace {

const int kRate = 16000;
const int kBlock = 320;           // 20 ms, the daemon's frame
const int kChunk = kRate / 100;   // 10 ms, WebRTC's frame

bool readAll(int fd, void* buf, size_t n) {
	auto* p = static_cast<uint8_t*>(buf);
	while (n > 0) {
		ssize_t r = read(fd, p, n);
		if (r == 0) return false;
		if (r < 0) {
			if (errno == EINTR) continue;
			return false;
		}
		p += r;
		n -= r;
	}
	return true;
}

bool writeAll(int fd, const void* buf, size_t n) {
	auto* p = static_cast<const uint8_t*>(buf);
	while (n > 0) {
		ssize_t w = write(fd, p, n);
		if (w < 0) {
			if (errno == EINTR) continue;
			return false;
		}
		p += w;
		n -= w;
	}
	return true;
}

}  // namespace

int main(int argc, char** argv) {
	std::string aec = "low", ns = "low";
	float gainDb = 0;
	for (int i = 1; i < argc; i++) {
		std::string a = argv[i];
		if (a == "--aec" && i + 1 < argc) aec = argv[++i];
		else if (a == "--ns" && i + 1 < argc) ns = argv[++i];
		else if (a == "--gain-db" && i + 1 < argc) gainDb = std::atof(argv[++i]);
		else {
			std::fprintf(stderr, "techo5-aec: unknown argument %s\n", argv[i]);
			return 2;
		}
	}

	std::unique_ptr<webrtc::AudioProcessing> apm(webrtc::AudioProcessingBuilder().Create());
	if (!apm) {
		std::fprintf(stderr, "techo5-aec: AudioProcessing failed to build\n");
		return 1;
	}
	webrtc::AudioProcessing::Config cfg;
	cfg.echo_canceller.enabled = true;
	cfg.echo_canceller.mobile_mode = false;  // the full linear canceller, not the mobile suppressor
	cfg.high_pass_filter.enabled = true;
	cfg.noise_suppression.enabled = ns != "off";
	if (ns == "low") cfg.noise_suppression.level = webrtc::AudioProcessing::Config::NoiseSuppression::kLow;
	else if (ns == "moderate") cfg.noise_suppression.level = webrtc::AudioProcessing::Config::NoiseSuppression::kModerate;
	else if (ns == "high") cfg.noise_suppression.level = webrtc::AudioProcessing::Config::NoiseSuppression::kHigh;
	else if (ns == "veryhigh") cfg.noise_suppression.level = webrtc::AudioProcessing::Config::NoiseSuppression::kVeryHigh;
	// A digital gain after cancellation, where a boost belongs: an analog boost clips the echo
	// before the canceller sees it.
	cfg.gain_controller2.enabled = gainDb != 0;
	cfg.gain_controller2.fixed_digital.gain_db = gainDb;
	apm->ApplyConfig(cfg);
	apm->set_stream_delay_ms(0);
	(void)aec;  // suppression level is fixed at the library's default in this API version

	webrtc::StreamConfig sc(kRate, 1, false);
	int16_t mic[kBlock], far[kBlock];
	int16_t out[kBlock];
	std::fprintf(stderr, "techo5-aec: ready (ns=%s gain=%.0f dB)\n", ns.c_str(), gainDb);
	while (readAll(0, mic, sizeof mic) && readAll(0, far, sizeof far)) {
		for (int off = 0; off < kBlock; off += kChunk) {
			// The far end first, so the forward pass sees what just played.
			apm->ProcessReverseStream(far + off, sc, sc, far + off);
			apm->set_stream_delay_ms(0);
			apm->ProcessStream(mic + off, sc, sc, out + off);
		}
		if (!writeAll(1, out, sizeof out)) break;
	}
	return 0;
}
