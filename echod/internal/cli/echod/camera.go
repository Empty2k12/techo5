package echod

import (
	"context"
	"errors"
	"fmt"
	"image/png"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/HuskerMinion/techo5/echod/internal/hardware/camera"
)

func newCameraCmd() *cobra.Command {
	var full bool
	c := &cobra.Command{
		Use:   "camera [out.png]",
		Short: "Take one picture and write it as a PNG",
		Long: "Acquires the sensor, waits for a frame and writes it out. The sensor's own line and\n" +
			"frame lengths are logged as it opens, which is how a new board's geometry is checked\n" +
			"against the table in hardware/camera.\n\n" +
			"The daemon holds the camera while it runs: stop it first (touch /run/techo5/hold and\n" +
			"wait for it to exit) or take the picture from the daemon instead. A muted unit keeps\n" +
			"the sensor powered off, so this times out until the mute is released.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !camera.Available() {
				return errors.New("no camera on this device")
			}
			out := "/tmp/camera.png"
			if len(args) == 1 {
				out = args[0]
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			f, err := camera.Get().Snapshot(ctx)
			if err != nil {
				return err
			}
			img := f.RGBA
			if full {
				img = f.Full()
			}
			w, err := os.Create(out)
			if err != nil {
				return err
			}
			defer w.Close()
			if err := png.Encode(w, img); err != nil {
				return err
			}
			b := img.Bounds()
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %dx%d\n", out, b.Dx(), b.Dy())
			return nil
		},
	}
	c.Flags().BoolVar(&full, "full", false, "the sensor's own size, demosaiced, instead of the half-size picture")
	return c
}
