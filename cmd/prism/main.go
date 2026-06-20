package main

import (
	"context"
	"fmt"
	"html/template"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/openana/prism/pkg/config"
	"github.com/openana/prism/pkg/meta"
	"github.com/openana/prism/pkg/server"
	"github.com/openana/prism/pkg/web"
	"github.com/rs/zerolog"
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

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Test configuration and news without starting the server",
	RunE:  runTest,
}

func init() {
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(testCmd)

	runCmd.Flags().StringP("config", "c", "config.yaml", "path to YAML config file")
	runCmd.Flags().StringP("log-level", "l", "", "override log.level")
	runCmd.Flags().String("log-output", "", "override log.output")
	runCmd.Flags().String("access-log-level", "", "override access_log.level")
	runCmd.Flags().String("access-log-output", "", "override access_log.output")

	testCmd.Flags().StringP("config", "c", "config.yaml", "path to YAML config file")
	testCmd.Flags().BoolP("all", "a", false, "run all tests (config + news)")
	testCmd.Flags().Bool("test-config", false, "validate configuration")
	testCmd.Flags().String("test-news", "", "validate news files in the given directory")
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

// test

func runTest(cmd *cobra.Command, args []string) error {
	configPath, _ := cmd.Flags().GetString("config")
	all, _ := cmd.Flags().GetBool("all")
	testConfig, _ := cmd.Flags().GetBool("test-config")
	testNewsPath, _ := cmd.Flags().GetString("test-news")

	if !all && !testConfig && testNewsPath == "" {
		return cmd.Help()
	}

	if all {
		testConfig = true
	}

	var cfg *config.Config
	configFailed := false
	newsFailed := false

	if testConfig {
		var err error
		cfg, err = config.Load(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "config.Load(%q): %v\n", configPath, err)
			configFailed = true
		} else {
			fmt.Println(meta.VersionString)
			if err := runTestConfig(cfg); err != nil {
				configFailed = true
			}
		}
	}

	newsDir := testNewsPath
	if all && newsDir == "" && cfg != nil {
		newsDir = cfg.News.Dir
	}
	if testNewsPath != "" || (all && newsDir != "") {
		if err := runTestNews(newsDir); err != nil {
			newsFailed = true
		}
	} else if all && newsDir == "" {
		fmt.Println("news.dir not set; skipping news test")
	}

	if configFailed || newsFailed {
		return fmt.Errorf("one or more tests failed")
	}
	fmt.Println("all tests passed")
	return nil
}

func runTestConfig(cfg *config.Config) error {
	var failed int
	ok := func(name string) {
		fmt.Printf("[OK] %s\n", name)
	}
	fail := func(name string, err error) {
		fmt.Fprintf(os.Stderr, "[FAIL] %s: %v\n", name, err)
		failed++
	}

	if _, err := cfg.Log.ToLogger(); err != nil {
		fail("log.ToLogger", err)
	} else {
		ok("log.ToLogger")
	}

	if _, err := cfg.AccessLog.ToLogger(); err != nil {
		fail("access_log.ToLogger", err)
	} else {
		ok("access_log.ToLogger")
	}

	if _, err := cfg.ToServer(); err != nil {
		fail("ToServer", err)
	} else {
		ok("ToServer")
	}

	_ = cfg.ToRouter()

	if _, err := cfg.ToCachedProvider(); err != nil {
		fail("ToCachedProvider", err)
	} else {
		ok("ToCachedProvider")
	}

	if _, err := cfg.ToMirrorManager(); err != nil {
		fail("ToMirrorManager", err)
	} else {
		ok("ToMirrorManager")
	}

	_ = cfg.ToTrieResolver()
	ok("ToTrieResolver")

	_ = cfg.ToWebServer()
	ok("ToWebServer")

	if failed > 0 {
		fmt.Fprintf(os.Stderr, "\n%d config check(s) failed\n", failed)
		return fmt.Errorf("config validation failed")
	}
	fmt.Println("\nconfig OK")
	return nil
}

func runTestNews(dir string) error {
	fmt.Printf("news directory: %s\n", dir)

	logger := zerolog.New(os.Stderr).Level(zerolog.WarnLevel)
	articles, _, _, err := web.LoadNews(dir, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FAIL] LoadNews: %v\n", err)
		return err
	}

	articleCount := 0
	if articles != nil {
		articleCount = len(articles)
	}
	fmt.Printf("parsed %d news article(s)\n", articleCount)

	funcMap := template.FuncMap{
		"site":     func() any { return nil },
		"version":  func() string { return "test" },
		"catAlias": func(s string) string { return s },
	}
	if _, err := web.ParseNewsTemplate(funcMap); err != nil {
		fmt.Fprintf(os.Stderr, "[FAIL] ParseNewsTemplate: %v\n", err)
		return err
	}
	fmt.Println("news OK")
	return nil
}
