package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/openana/prism/pkg/config"
	"github.com/openana/prism/pkg/meta"
	"github.com/openana/prism/pkg/server"
	"github.com/spf13/cobra"
)

func main() {
	cobra.CheckErr(rootCmd.Execute())
}

var rootCmd = &cobra.Command{
	Use:           "prism",
	Short:         "Prism mirror gateway",
	SilenceUsage:  true,
	SilenceErrors: true,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(meta.VersionString)
	},
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the prism server",
	RunE:  runServer,
}

func init() {
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(versionCmd)

	runCmd.Flags().StringP("config", "c", "config.yaml", "path to YAML config file")
	runCmd.Flags().StringP("log-level", "l", "", "override log.level")
	runCmd.Flags().String("log-output", "", "override log.output")
	runCmd.Flags().String("access-log-level", "", "override access_log.level")
	runCmd.Flags().String("access-log-output", "", "override access_log.output")
}

func runServer(cmd *cobra.Command, args []string) error {
	defer initProfiles()()

	configPath, _ := cmd.Flags().GetString("config")

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Print version string
	fmt.Println(meta.VersionString)

	// Apply config overrides
	if cmd.Flags().Changed("log-level") {
		val, _ := cmd.Flags().GetString("log-level")
		cfg.Log.Level = val
	}
	if cmd.Flags().Changed("log-output") {
		val, _ := cmd.Flags().GetString("log-output")
		cfg.Log.Output = val
	}
	if cmd.Flags().Changed("access-log-level") {
		val, _ := cmd.Flags().GetString("access-log-level")
		cfg.AccessLog.Level = val
	}
	if cmd.Flags().Changed("access-log-output") {
		val, _ := cmd.Flags().GetString("access-log-output")
		cfg.AccessLog.Output = val
	}

	srv, cleanup, err := server.InitializeServer(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize server: %w", err)
	}
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Run(ctx); err != nil {
		return fmt.Errorf("server run error: %w", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Stop(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "server stop error: %v\n", err)
	}

	return nil
}
