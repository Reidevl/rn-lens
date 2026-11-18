//go:build server

package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	"github.com/Reidevl/rn-lens/internal/adapters/wailsui"
)

// defaultUIPort is where browser mode serves the UI; 7420 is the SDK's.
const defaultUIPort = 7430

const runUsage = `  rnlens                 Open Lens in your browser (http://localhost:7430)
    --port N             Port for the Lens UI (default 7430)
    --host H             Address to listen on (default localhost; anything
                         else exposes your app's data to that network)
    --no-open            Print the URL without opening a browser
`

type modeFlags struct {
	host   *string
	port   *int
	noOpen *bool
}

func registerModeFlags(fs *flag.FlagSet) modeFlags {
	return modeFlags{
		host:   fs.String("host", "localhost", "address the Lens UI listens on"),
		port:   fs.Int("port", defaultUIPort, "port the Lens UI listens on"),
		noOpen: fs.Bool("no-open", false, "do not open a browser"),
	}
}

func (m modeFlags) options() wailsui.RunOptions {
	return wailsui.RunOptions{Host: *m.host, Port: *m.port}
}

// start checks the port, prints the URL and opens it once the server
// accepts connections.
func (m modeFlags) start() error {
	if err := checkPort(*m.host, *m.port); err != nil {
		return err
	}
	host := *m.host
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	url := "http://" + net.JoinHostPort(host, strconv.Itoa(*m.port))
	fmt.Fprintf(os.Stderr, "Lens is running at %s (Ctrl+C to quit)\n", url)
	if *m.host != "localhost" && *m.host != "127.0.0.1" && *m.host != "::1" {
		fmt.Fprintf(os.Stderr, "warning: listening on %s; anyone who can reach it can read your app's data\n", *m.host)
	}
	if *m.noOpen {
		return nil
	}
	go func() {
		addr := net.JoinHostPort(host, strconv.Itoa(*m.port))
		for range 100 {
			d := net.Dialer{Timeout: 100 * time.Millisecond}
			if c, err := d.DialContext(context.Background(), "tcp", addr); err == nil {
				_ = c.Close()
				_ = openBrowser(url)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()
	return nil
}

// checkPort fails when something already answers on the port, or the port
// cannot be bound. Answering counts because on macOS a bind to localhost can
// succeed next to a process holding the port on every IPv6 address.
func checkPort(host string, port int) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid port %d", port)
	}
	d := net.Dialer{Timeout: 200 * time.Millisecond}
	for _, h := range []string{"127.0.0.1", "::1"} {
		if c, err := d.DialContext(context.Background(), "tcp", net.JoinHostPort(h, strconv.Itoa(port))); err == nil {
			_ = c.Close()
			return fmt.Errorf("port %d is already in use; pick another with --port", port)
		}
	}
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w", net.JoinHostPort(host, strconv.Itoa(port)), err)
	}
	return ln.Close()
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(context.Background(), "open", url)
	case "windows":
		cmd = exec.CommandContext(context.Background(), "rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.CommandContext(context.Background(), "xdg-open", url)
	}
	return cmd.Start()
}
