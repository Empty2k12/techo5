//go:build linux

// Package btaudio plays through Bluetooth earbuds or a speaker. bluetoothd owns the radio and the
// bonds, bluez-alsa owns the A2DP stream; this feature pairs and connects the device, and while it
// is connected hands the speaker's audio to bluez-alsa instead of the codec.
//
// Pairing is a mode the user turns on — from the screen, from Home Assistant — and it ends on its
// own: while it is on the radio scans, and scanning shares the antenna with Wi-Fi. What the scan
// finds is listed on the screen; a tap pairs, trusts and connects it. A trusted device reconnects
// on its own when it is switched on, and the daemon tries the last one once at start-up.
//
// To Home Assistant: a switch for pairing mode, a sensor saying what is connected, and buttons to
// reconnect or disconnect the remembered device.
package btaudio

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	esphome "github.com/ygelfand/go-esphome-device"

	"github.com/HuskerMinion/techo5/echod/internal/component"
	"github.com/HuskerMinion/techo5/echod/internal/config"
	"github.com/HuskerMinion/techo5/echod/internal/hardware/speaker"
	"github.com/HuskerMinion/techo5/echod/internal/layout"
	"github.com/HuskerMinion/techo5/echod/internal/lib/bluealsa"
	"github.com/HuskerMinion/techo5/echod/internal/lib/bluez"
	"github.com/HuskerMinion/techo5/echod/internal/lib/hook"
	"github.com/HuskerMinion/techo5/echod/internal/lib/safe"
	"github.com/HuskerMinion/techo5/echod/internal/service"
)

func init() {
	if layout.OnAndroid() {
		return // Android's own stack owns the radio there
	}
	component.Register(component.Device, Get(), component.Order(70),
		component.Supervise(service.Restart(2*time.Second, 30*time.Second)))
}

const (
	// pairingFor is how long pairing mode stays on by itself.
	pairingFor = 3 * time.Minute

	// connectTimeout bounds one attempt; earbuds that are in their case never answer.
	connectTimeout = 20 * time.Second

	notConnected = "Not connected"
)

// Device is one entry on the screen's list.
type Device struct {
	Address   string
	Name      string
	RSSI      int
	Paired    bool
	Connected bool
	Busy      bool // being paired or connected right now
}

// State is what the screen shows.
type State struct {
	Available bool // bluetoothd is there
	Pairing   bool
	Devices   []Device // while pairing: what the scan found that could play audio
	Connected string   // the connected audio device's name, empty for none
	Remembered string  // the last device's name, for "waiting for …"
	Status    string   // one line: what just happened
}

type Feature struct {
	pairing    *esphome.Switch
	status     *esphome.TextSensor
	reconnect  *esphome.Button
	disconnect *esphome.Button

	// Changed fires when State does; listeners must not block.
	Changed hook.Hook[State]

	mu      sync.Mutex
	adapter *bluez.Adapter
	alsa    *bluealsa.Client
	stream  *bluealsa.Stream
	state   State
	busy    map[string]bool
	pairOff *time.Timer
	poke    chan struct{}

	// refusals counts stream opens bluetoothd refused for the current connection; see attach.
	refusals int
}

var (
	once   sync.Once
	shared *Feature
)

func Get() *Feature {
	once.Do(func() { shared = build() })
	return shared
}

func build() *Feature {
	f := &Feature{
		pairing: &esphome.Switch{Base: esphome.Base{
			ObjectID: "bluetooth_pairing", Name: "Bluetooth pairing", Icon: "mdi:bluetooth-settings",
			Category: esphome.CategoryConfig,
		}},
		status: &esphome.TextSensor{Base: esphome.Base{
			ObjectID: "bluetooth_audio", Name: "Bluetooth audio", Icon: "mdi:headphones",
		}},
		reconnect: &esphome.Button{Base: esphome.Base{
			ObjectID: "bluetooth_reconnect", Name: "Bluetooth reconnect", Icon: "mdi:bluetooth-connect",
			Category: esphome.CategoryConfig,
		}},
		disconnect: &esphome.Button{Base: esphome.Base{
			ObjectID: "bluetooth_disconnect", Name: "Bluetooth disconnect", Icon: "mdi:bluetooth-off",
			Category: esphome.CategoryConfig,
		}},
		busy:  map[string]bool{},
		poke:  make(chan struct{}, 1),
		state: State{Status: notConnected},
	}
	f.pairing.OnCommand = func(on bool) { f.SetPairing(on) }
	f.reconnect.OnPress = func() { safe.Go("bluetooth reconnect", f.Reconnect) }
	f.disconnect.OnPress = func() { safe.Go("bluetooth disconnect", f.Disconnect) }
	f.status.Set(notConnected)
	return f
}

func (f *Feature) Name() string { return "bluetooth audio" }

func (f *Feature) Entities() []esphome.Entity {
	return []esphome.Entity{f.pairing, f.status, f.reconnect, f.disconnect}
}

// State is a copy of what the screen shows.
func (f *Feature) State() State {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.state
	s.Devices = append([]Device(nil), f.state.Devices...)
	return s
}

func (f *Feature) wake() {
	select {
	case f.poke <- struct{}{}:
	default:
	}
}

// Start connects to bluetoothd and bluez-alsa. Both come up after the daemon on a fresh boot, so
// failing here is expected for a while; the supervisor tries again.
func (f *Feature) Start(ctx context.Context) error {
	a, err := bluez.Open(ctx)
	if err != nil {
		return err
	}
	if err := a.RegisterAgent(); err != nil {
		a.Close()
		return err
	}
	al, err := bluealsa.Open(ctx)
	if err != nil {
		a.Close()
		return err
	}
	if name := config.Get().Device.Name; name != "" {
		if err := a.Alias(name); err != nil {
			slog.Warn("bluetooth alias", "err", err)
		}
	}
	// Not findable until someone asks: bluetoothd remembers discoverable across restarts.
	if err := a.Pairing(false); err != nil {
		slog.Warn("bluetooth: leaving pairing mode", "err", err)
	}
	_ = a.Discover(false)
	f.mu.Lock()
	f.adapter, f.alsa = a, al
	f.state.Available = true
	f.state.Remembered = config.Get().Bluetooth.AudioName
	f.mu.Unlock()
	a.Changed.Listen(func(struct{}) { f.wake() })
	al.Changed.Listen(func(struct{}) { f.wake() })
	slog.Info("bluetooth up", "address", a.Address(), "devices", len(a.Devices()))
	return nil
}

func (f *Feature) Close() error {
	f.mu.Lock()
	a, al := f.adapter, f.alsa
	f.adapter, f.alsa = nil, nil
	f.state.Available = false
	f.mu.Unlock()
	f.detach()
	if al != nil {
		al.Close()
	}
	if a != nil {
		a.Close()
	}
	return nil
}

// Run follows the devices and streams: attaches the speaker to an A2DP stream when one appears,
// lets it go when it does, and keeps the screen's list current.
func (f *Feature) Run(ctx context.Context) error {
	safe.Go("bluetooth first connect", func() { f.tryRemembered(ctx) })
	for {
		f.refresh()
		select {
		case <-ctx.Done():
			return nil
		case <-f.poke:
		case <-time.After(5 * time.Second):
		}
	}
}

// tryRemembered gives the last device one chance at start-up. Earbuds in their case do not answer
// and that is fine.
func (f *Feature) tryRemembered(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(3 * time.Second):
	}
	addr := f.remembered()
	if addr == "" {
		return
	}
	f.connect(ctx, addr, false)
}

// remembered is the device to reach for: the one saved here, or failing that the first audio
// device bluetoothd holds a bond for — one paired from the console, or before this build.
func (f *Feature) remembered() string {
	if addr := config.Get().Bluetooth.Audio; addr != "" {
		return addr
	}
	f.mu.Lock()
	a := f.adapter
	f.mu.Unlock()
	if a == nil {
		return ""
	}
	for _, d := range a.Devices() {
		if d.Paired && d.AudioSink {
			return d.Address
		}
	}
	return ""
}

// refresh reconciles: the stream the speaker should be on, and what the screen should say.
func (f *Feature) refresh() {
	f.mu.Lock()
	a, al := f.adapter, f.alsa
	f.mu.Unlock()
	if a == nil || al == nil {
		return
	}

	devices := a.Devices()
	byPath := map[string]bluez.Device{}
	for _, d := range devices {
		byPath[string(d.Path)] = d
	}

	// The stream: the first playable A2DP PCM whose device is connected.
	var want *bluealsa.PCM
	var wantDev bluez.Device
	for _, p := range al.PCMs() {
		if !p.Playable() {
			continue
		}
		d, ok := byPath[string(p.Device)]
		if !ok || !d.Connected {
			continue
		}
		pc := p
		want, wantDev = &pc, d
		break
	}

	f.mu.Lock()
	cur := f.stream
	f.mu.Unlock()
	switch {
	case want == nil && cur != nil:
		f.detach()
	case want == nil:
		f.mu.Lock()
		f.refusals = 0 // whatever comes next is a new connection
		f.mu.Unlock()
	case want != nil && (cur == nil || cur.Path != want.Path):
		f.attach(al, *want, wantDev)
	}

	// The screen and Home Assistant.
	f.mu.Lock()
	st := &f.state
	st.Connected = ""
	if f.stream != nil {
		if d, ok := byPath[string(f.stream.Device)]; ok {
			st.Connected = displayName(d)
		}
	}
	st.Devices = st.Devices[:0]
	if st.Pairing {
		for _, d := range devices {
			if !d.AudioSink && d.Icon != "audio-headset" && d.Icon != "audio-headphones" && d.Icon != "audio-card" {
				continue
			}
			if d.Name == "" {
				continue
			}
			st.Devices = append(st.Devices, Device{
				Address: d.Address, Name: displayName(d), RSSI: int(d.RSSI),
				Paired: d.Paired, Connected: d.Connected, Busy: f.busy[strings.ToLower(d.Address)],
			})
		}
		sort.SliceStable(st.Devices, func(i, j int) bool { return st.Devices[i].RSSI > st.Devices[j].RSSI })
	}
	text := notConnected
	if st.Connected != "" {
		text = st.Connected
	}
	snapshot := *st
	snapshot.Devices = append([]Device(nil), st.Devices...)
	f.mu.Unlock()

	if f.status.Get() != text {
		f.status.Set(text)
	}
	f.Changed.Emit(snapshot)
}

func displayName(d bluez.Device) string {
	if d.Name != "" {
		return d.Name
	}
	return d.Address
}

// attach opens the stream and points the speaker at it.
//
// A stream the device set up itself — earbuds coming out of their case connect on their own —
// cannot be acquired from this side: bluetoothd answers NotAuthorized, and keeps answering. What
// works is a connection this side made, so after a couple of refusals the link is dropped and
// made again, once per connection.
func (f *Feature) attach(al *bluealsa.Client, p bluealsa.PCM, d bluez.Device) {
	s, err := al.Open(p)
	if err != nil {
		slog.Warn("bluetooth stream", "device", displayName(d), "err", err)
		f.mu.Lock()
		f.refusals++
		n := f.refusals
		f.mu.Unlock()
		if n == 2 {
			slog.Info("bluetooth: remaking the connection ourselves", "device", displayName(d))
			safe.Go("bluetooth remake", func() {
				ctx, cancel := context.WithTimeout(context.Background(), 2*connectTimeout)
				defer cancel()
				f.mu.Lock()
				a := f.adapter
				f.mu.Unlock()
				if a == nil {
					return
				}
				if err := a.Disconnect(ctx, d.Address); err != nil {
					slog.Warn("bluetooth disconnect", "device", displayName(d), "err", err)
				}
				time.Sleep(2 * time.Second)
				f.connect(ctx, d.Address, false)
			})
		}
		return
	}
	f.mu.Lock()
	f.refusals = 0
	f.mu.Unlock()
	f.mu.Lock()
	old := f.stream
	f.stream = s
	f.mu.Unlock()
	if old != nil {
		speaker.Get().SetSink(nil, "", 0, 0) // closes it
	}
	speaker.Get().SetSink(s, displayName(d), p.Rate, p.Channels)
	slog.Info("playing through bluetooth", "device", displayName(d), "codec", p.Codec, "rate", p.Rate, "channels", p.Channels)
	f.note(displayName(d) + " connected")
}

// detach puts the speaker back on the codec.
func (f *Feature) detach() {
	f.mu.Lock()
	s := f.stream
	f.stream = nil
	f.refusals = 0
	f.mu.Unlock()
	if s == nil {
		return
	}
	speaker.Get().SetSink(nil, "", 0, 0) // the speaker closes the stream
	slog.Info("bluetooth audio ended")
	f.note("Disconnected")
}

func (f *Feature) note(s string) {
	f.mu.Lock()
	f.state.Status = s
	f.mu.Unlock()
}

// SetPairing turns pairing mode on or off: discoverable, pairable and scanning while on, and off
// again on its own after a while.
func (f *Feature) SetPairing(on bool) {
	f.mu.Lock()
	a := f.adapter
	if f.pairOff != nil {
		f.pairOff.Stop()
		f.pairOff = nil
	}
	f.state.Pairing = on && a != nil
	if on && a != nil {
		f.pairOff = time.AfterFunc(pairingFor, func() { f.SetPairing(false) })
		f.state.Status = "Put your earbuds in pairing mode"
	}
	f.mu.Unlock()
	f.pairing.Set(on && a != nil)
	if a == nil {
		slog.Warn("bluetooth pairing asked for, no adapter")
		return
	}
	if err := a.Pairing(on); err != nil {
		slog.Warn("bluetooth pairing mode", "on", on, "err", err)
	}
	if err := a.Discover(on); err != nil {
		slog.Warn("bluetooth discovery", "on", on, "err", err)
	}
	slog.Info("bluetooth pairing mode", "on", on)
	f.wake()
}

// Pairing reports whether pairing mode is on.
func (f *Feature) Pairing() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state.Pairing
}

// Choose is a tap on a listed device: pair it if it is not, then connect. Runs in the background.
func (f *Feature) Choose(address string) {
	safe.Go("bluetooth choose", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*connectTimeout)
		defer cancel()
		f.connect(ctx, address, true)
	})
}

func (f *Feature) setBusy(address string, on bool) {
	f.mu.Lock()
	if on {
		f.busy[strings.ToLower(address)] = true
	} else {
		delete(f.busy, strings.ToLower(address))
	}
	f.mu.Unlock()
	f.wake()
}

// connect pairs (when asked) and connects one device, remembers it, and ends pairing mode on
// success.
func (f *Feature) connect(ctx context.Context, address string, pair bool) {
	f.mu.Lock()
	a := f.adapter
	f.mu.Unlock()
	if a == nil {
		return
	}
	d, ok := a.Device(address)
	if !ok {
		slog.Warn("bluetooth connect: unknown device", "address", address)
		return
	}
	name := displayName(d)
	f.setBusy(address, true)
	defer f.setBusy(address, false)

	if pair && !d.Paired {
		f.note("Pairing " + name + "…")
		if err := a.Pair(ctx, address); err != nil {
			slog.Warn("bluetooth pair", "device", name, "err", err)
			f.note("Pairing " + name + " failed")
			return
		}
		if err := a.Trust(address, true); err != nil {
			slog.Warn("bluetooth trust", "device", name, "err", err)
		}
	}
	f.note("Connecting " + name + "…")
	cctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	if err := a.Connect(cctx, address); err != nil {
		if strings.Contains(err.Error(), "Already Connected") {
			err = nil
		} else {
			slog.Warn("bluetooth connect", "device", name, "err", err)
			f.note("Could not connect " + name)
			return
		}
	}
	if err := config.Set().Bluetooth().Audio(address, name); err != nil {
		slog.Error("saving the bluetooth device failed", "err", err)
	}
	f.mu.Lock()
	f.state.Remembered = name
	f.mu.Unlock()
	slog.Info("bluetooth connected", "device", name)
	if pair {
		f.SetPairing(false)
	}
	f.wake()
}

// Reconnect tries the remembered device now.
func (f *Feature) Reconnect() {
	addr := f.remembered()
	if addr == "" {
		slog.Info("bluetooth reconnect: no device to reach for")
		f.note("No device remembered")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	f.connect(ctx, addr, false)
}

// Disconnect drops the connected audio device; it stays paired and trusted.
func (f *Feature) Disconnect() {
	f.mu.Lock()
	a := f.adapter
	f.mu.Unlock()
	if a == nil {
		return
	}
	for _, d := range a.Devices() {
		if d.Connected && d.AudioSink {
			ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
			err := a.Disconnect(ctx, d.Address)
			cancel()
			if err != nil {
				slog.Warn("bluetooth disconnect", "device", displayName(d), "err", err)
			}
		}
	}
	f.wake()
}

// Forget removes the remembered device's bond.
func (f *Feature) Forget() {
	f.mu.Lock()
	a := f.adapter
	f.mu.Unlock()
	addr := config.Get().Bluetooth.Audio
	if a != nil && addr != "" {
		if err := a.Remove(addr); err != nil {
			slog.Warn("bluetooth forget", "err", err)
		}
	}
	_ = config.Set().Bluetooth().Audio("", "")
	f.mu.Lock()
	f.state.Remembered = ""
	f.mu.Unlock()
	f.wake()
}
