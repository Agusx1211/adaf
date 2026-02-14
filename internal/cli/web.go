package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/session"
	"github.com/agusx1211/adaf/internal/webserver"
)

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Start the web server",
	Long:  `Start an HTTP/WebSocket server exposing project data and session streaming.`,
	RunE:  runWeb,
}

func init() {
	webCmd.Flags().IntP("port", "p", 8080, "Port to listen on")
	webCmd.Flags().String("host", "127.0.0.1", "Host to bind to")
	rootCmd.AddCommand(webCmd)
}

func runWeb(cmd *cobra.Command, args []string) error {
	if session.IsAgentContext() {
		return fmt.Errorf("web is not available inside an agent context")
	}

	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	host, _ := cmd.Flags().GetString("host")
	port, _ := cmd.Flags().GetInt("port")

	srv := webserver.New(s, host, port)
	if err := srv.Start(); err != nil {
		return fmt.Errorf("starting web server: %w", err)
	}

	fmt.Printf("Web server running at http://%s\n", srv.Addr())

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutting down web server: %w", err)
	}

	return nil
}
