package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// TestStep represents a single test step
type TestStep struct {
	Phase     string    `yaml:"phase"`
	Status    string    `yaml:"status"`
	Message   string    `yaml:"message"`
	Timestamp time.Time `yaml:"timestamp"`
}

// TestReport represents the complete test report
type TestReport struct {
	TestRun struct {
		StartTime   time.Time `yaml:"start_time"`
		Runner      string    `yaml:"runner"`
		ProjectName string    `yaml:"project_name,omitempty"`
	} `yaml:"test_run"`
	Environment struct {
		OS    string `yaml:"os"`
		Arch  string `yaml:"arch"`
		Shell string `yaml:"shell"`
	} `yaml:"environment"`
	Steps   []TestStep `yaml:"steps"`
	Summary *struct {
		EndTime       time.Time `yaml:"end_time"`
		Duration      int       `yaml:"duration_seconds"`
		TotalSteps    int       `yaml:"total_steps"`
		PassedSteps   int       `yaml:"passed_steps"`
		FailedSteps   int       `yaml:"failed_steps"`
		OverallStatus string    `yaml:"overall_status"`
		SuccessRate   string    `yaml:"success_rate"`
	} `yaml:"summary,omitempty"`
}

// GitHubPayload represents the GitHub Actions client payload structure
type GitHubPayload struct {
	Success      bool                       `json:"success"`
	Source       string                     `json:"source"`
	Branch       string                     `json:"branch"`
	Commit       string                     `json:"commit"`
	ArtifactName string                     `json:"artifact_name"`
	Artifacts    map[string]ArtifactContent `json:"artifacts"`
}

// ArtifactContent represents base64 encoded artifact content
type ArtifactContent struct {
	Content string `json:"content"`
}

var (
	reportFile string
	verbosity  int
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "home-ci-reporter",
		Short: "E2E test report generator",
		Long:  "Generates YAML test reports for e2e tests with atomic operations ensuring valid YAML at all times",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			setupLogging(verbosity)
		},
	}

	var initCmd = &cobra.Command{
		Use:   "init <report-file> [project-name]",
		Short: "Initialize a new test report",
		Args:  cobra.RangeArgs(1, 2),
		RunE:  initReport,
	}

	var stepCmd = &cobra.Command{
		Use:   "step <phase> <status> <message>",
		Short: "Add a test step result",
		Args:  cobra.ExactArgs(3),
		RunE:  addStep,
	}

	var finalizeCmd = &cobra.Command{
		Use:   "finalize",
		Short: "Finalize the test report with summary",
		RunE:  finalizeReport,
	}

	var parseCmd = &cobra.Command{
		Use:   "parse <report-file>",
		Short: "Parse and display test report with GitHub Actions formatting",
		Args:  cobra.ExactArgs(1),
		RunE:  parseReport,
	}

	var extractCmd = &cobra.Command{
		Use:   "extract <payload-json> <output-dir>",
		Short: "Extract and decode artifacts from GitHub Actions payload",
		Args:  cobra.ExactArgs(2),
		RunE:  extractArtifacts,
	}

	var summaryCmd = &cobra.Command{
		Use:   "summary <payload-json>",
		Short: "Generate execution summary from GitHub Actions payload",
		Args:  cobra.ExactArgs(1),
		RunE:  generateSummary,
	}

	stepCmd.Flags().StringVarP(&reportFile, "file", "f", "", "Report file path (required)")
	stepCmd.MarkFlagRequired("file")

	finalizeCmd.Flags().StringVarP(&reportFile, "file", "f", "", "Report file path (required)")
	finalizeCmd.MarkFlagRequired("file")

	rootCmd.PersistentFlags().CountVarP(&verbosity, "verbose", "v", "Increase verbosity (use -v, -vv, -vvv for different levels)")

	rootCmd.AddCommand(initCmd, stepCmd, finalizeCmd, parseCmd, extractCmd, summaryCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// setupLogging configures slog based on verbosity level
func setupLogging(verbosity int) {
	var level slog.Level

	switch verbosity {
	case 0:
		level = slog.LevelWarn  // Default: only warnings and errors
	case 1:
		level = slog.LevelInfo  // -v: info, warnings, and errors
	case 2:
		level = slog.LevelDebug // -vv: debug and above
	default:
		level = slog.LevelDebug // -vvv+: debug and above (same as -vv)
	}

	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})

	logger := slog.New(handler)
	slog.SetDefault(logger)

	slog.Debug("Logging initialized", "verbosity_level", verbosity, "slog_level", level)
}

func initReport(cmd *cobra.Command, args []string) error {
	reportPath := args[0]
	projectName := ""
	if len(args) > 1 {
		projectName = args[1]
	}

	slog.Info("Initializing report", "report_path", reportPath, "project_name", projectName)

	// Create directory if needed
	if err := os.MkdirAll(filepath.Dir(reportPath), 0755); err != nil {
		slog.Error("Failed to create directory", "error", err, "path", filepath.Dir(reportPath))
		return fmt.Errorf("failed to create directory: %w", err)
	}

	hostname, _ := os.Hostname()

	report := TestReport{
		Steps: make([]TestStep, 0),
	}

	report.TestRun.StartTime = time.Now().UTC()
	report.TestRun.Runner = hostname
	if projectName != "" {
		report.TestRun.ProjectName = projectName
	}

	// Get environment info
	report.Environment.OS = getEnvOrDefault("GOOS", "unknown")
	report.Environment.Arch = getEnvOrDefault("GOARCH", "unknown")
	report.Environment.Shell = os.Args[0]

	slog.Debug("Report environment", "os", report.Environment.OS, "arch", report.Environment.Arch, "shell", report.Environment.Shell, "hostname", hostname)

	return writeReport(reportPath, report)
}

func addStep(cmd *cobra.Command, args []string) error {
	phase := args[0]
	status := args[1]
	message := args[2]

	slog.Info("Adding step", "phase", phase, "status", status, "message", message, "report_file", reportFile)

	report, err := readReport(reportFile)
	if err != nil {
		slog.Error("Failed to read report", "error", err, "file", reportFile)
		return err
	}

	step := TestStep{
		Phase:     phase,
		Status:    status,
		Message:   message,
		Timestamp: time.Now().UTC(),
	}

	report.Steps = append(report.Steps, step)

	return writeReport(reportFile, *report)
}

func finalizeReport(cmd *cobra.Command, args []string) error {
	report, err := readReport(reportFile)
	if err != nil {
		return err
	}

	endTime := time.Now().UTC()
	duration := int(endTime.Sub(report.TestRun.StartTime).Seconds())

	totalSteps := len(report.Steps)
	passedSteps := 0
	failedSteps := 0

	for _, step := range report.Steps {
		switch step.Status {
		case "passed":
			passedSteps++
		case "failed":
			failedSteps++
		}
	}

	overallStatus := "passed"
	if failedSteps > 0 {
		overallStatus = "failed"
	}

	successRate := "0%"
	if totalSteps > 0 {
		successRate = fmt.Sprintf("%.0f%%", float64(passedSteps)/float64(totalSteps)*100)
	}

	report.Summary = &struct {
		EndTime       time.Time `yaml:"end_time"`
		Duration      int       `yaml:"duration_seconds"`
		TotalSteps    int       `yaml:"total_steps"`
		PassedSteps   int       `yaml:"passed_steps"`
		FailedSteps   int       `yaml:"failed_steps"`
		OverallStatus string    `yaml:"overall_status"`
		SuccessRate   string    `yaml:"success_rate"`
	}{
		EndTime:       endTime,
		Duration:      duration,
		TotalSteps:    totalSteps,
		PassedSteps:   passedSteps,
		FailedSteps:   failedSteps,
		OverallStatus: overallStatus,
		SuccessRate:   successRate,
	}

	return writeReport(reportFile, *report)
}

func readReport(path string) (*TestReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read report file: %w", err)
	}

	var report TestReport
	if err := yaml.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return &report, nil
}

func writeReport(path string, report TestReport) error {
	// Write with proper YAML formatting and header comment
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create report file: %w", err)
	}
	defer file.Close()

	// Write header comment
	if _, err := file.WriteString("# E2E Test Report\n"); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Marshal and write YAML
	encoder := yaml.NewEncoder(file)
	encoder.SetIndent(2)
	defer encoder.Close()

	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("failed to encode YAML: %w", err)
	}

	return nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Debug function to print report as JSON for troubleshooting
func parseReport(cmd *cobra.Command, args []string) error {
	reportPath := args[0]

	slog.Info("Starting report parsing", "report_path", reportPath)

	report, err := readReport(reportPath)
	if err != nil {
		slog.Error("Failed to read report", "error", err, "path", reportPath)
		return fmt.Errorf("failed to read report: %w", err)
	}

	slog.Debug("Report loaded successfully", "steps_count", len(report.Steps), "has_summary", report.Summary != nil)

	// Get GitHub Actions step summary path from environment
	summaryPath := os.Getenv("GITHUB_STEP_SUMMARY")
	slog.Debug("Environment check", "GITHUB_STEP_SUMMARY", summaryPath, "GITHUB_ACTIONS", os.Getenv("GITHUB_ACTIONS"))

	if summaryPath == "" {
		// If not running in GitHub Actions, output to stdout
		slog.Info("Running in local mode - outputting to console")
		return outputReportToConsole(*report)
	}

	slog.Info("Running in GitHub Actions mode", "summary_file", summaryPath)
	return appendReportToGitHubSummary(*report, summaryPath)
}

func outputReportToConsole(report TestReport) error {
	fmt.Println("### 📊 Test Metrics")

	if report.Summary != nil {
		s := report.Summary
		fmt.Printf("- **Overall Status**: %s\n", s.OverallStatus)
		fmt.Printf("- **Success Rate**: %s\n", s.SuccessRate)
		fmt.Printf("- **Duration**: %ds\n", s.Duration)
	} else {
		// Calculate overall status from steps when summary is missing
		passedSteps := 0
		failedSteps := 0
		totalSteps := len(report.Steps)

		for _, step := range report.Steps {
			switch step.Status {
			case "passed":
				passedSteps++
			case "failed":
				failedSteps++
			}
		}

		overallStatus := "passed"
		if failedSteps > 0 {
			overallStatus = "failed"
		}

		var successRate string
		if totalSteps > 0 {
			successRate = fmt.Sprintf("%.0f%%", float64(passedSteps)/float64(totalSteps)*100)
		} else {
			successRate = "0%"
		}

		fmt.Printf("- **Overall Status**: %s\n", overallStatus)
		fmt.Printf("- **Success Rate**: %s\n", successRate)
		fmt.Println("- **Duration**: N/A")
	}

	// Always show detailed steps if they exist
	if len(report.Steps) > 0 {
		fmt.Println("\n#### 📋 Detailed Steps")
		for _, step := range report.Steps {
			fmt.Printf("- **%s**: %s _(%s)_\n", step.Phase, step.Status, step.Timestamp.Format(time.RFC3339))
		}
	}

	return nil
}

func appendReportToGitHubSummary(report TestReport, summaryPath string) error {
	slog.Debug("Opening GitHub summary file", "path", summaryPath)

	file, err := os.OpenFile(summaryPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		slog.Error("Failed to open GitHub summary file", "error", err, "path", summaryPath)
		return fmt.Errorf("failed to open GitHub summary file: %w", err)
	}
	defer file.Close()

	// Write test metrics section
	slog.Debug("Writing test metrics section to summary file")
	if _, err := file.WriteString("### 📊 Test Metrics\n"); err != nil {
		slog.Error("Failed to write metrics header", "error", err)
		return fmt.Errorf("failed to write to summary: %w", err)
	}

	if report.Summary != nil {
		s := report.Summary
		slog.Debug("Writing summary data", "overall_status", s.OverallStatus, "success_rate", s.SuccessRate, "duration", s.Duration)
		if _, err := fmt.Fprintf(file, "- **Overall Status**: %s\n", s.OverallStatus); err != nil {
			return fmt.Errorf("failed to write overall status: %w", err)
		}
		if _, err := fmt.Fprintf(file, "- **Success Rate**: %s\n", s.SuccessRate); err != nil {
			return fmt.Errorf("failed to write success rate: %w", err)
		}
		if _, err := fmt.Fprintf(file, "- **Duration**: %ds\n", s.Duration); err != nil {
			return fmt.Errorf("failed to write duration: %w", err)
		}

		// Write detailed steps
		if _, err := file.WriteString("\n#### 📋 Detailed Steps\n"); err != nil {
			return fmt.Errorf("failed to write steps header: %w", err)
		}

		for _, step := range report.Steps {
			if _, err := fmt.Fprintf(file, "- **%s**: %s _(%s)_\n",
				step.Phase,
				step.Status,
				step.Timestamp.Format(time.RFC3339)); err != nil {
				return fmt.Errorf("failed to write step: %w", err)
			}
		}

		slog.Debug("Successfully wrote summary to file", "steps_written", len(report.Steps))
	} else {
		slog.Warn("No summary data available in report")
		if _, err := file.WriteString("⚠️ No summary data available\n"); err != nil {
			return fmt.Errorf("failed to write no summary message: %w", err)
		}
	}

	slog.Info("Successfully appended report to GitHub summary")
	return nil
}

func debugPrint(report TestReport) {
	data, _ := json.MarshalIndent(report, "", "  ")
	fmt.Fprintf(os.Stderr, "DEBUG: %s\n", data)
}

// extractArtifacts extracts and decodes artifacts from GitHub Actions payload
func extractArtifacts(cmd *cobra.Command, args []string) error {
	payloadPath := args[0]
	outputDir := args[1]

	// Read and parse payload
	data, err := os.ReadFile(payloadPath)
	if err != nil {
		return fmt.Errorf("failed to read payload file: %w", err)
	}

	var payload GitHubPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("failed to parse payload JSON: %w", err)
	}

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	fmt.Println("📦 Extracting artifacts...")

	// Extract and decode artifacts
	for filename, artifact := range payload.Artifacts {
		if artifact.Content == "" || artifact.Content == "null" {
			fmt.Printf("⚠️  Skipping empty artifact: %s\n", filename)
			continue
		}

		// Decode base64 content
		content, err := base64.StdEncoding.DecodeString(artifact.Content)
		if err != nil {
			return fmt.Errorf("failed to decode artifact %s: %w", filename, err)
		}

		// Write to file
		outputPath := filepath.Join(outputDir, filename)
		if err := os.WriteFile(outputPath, content, 0644); err != nil {
			return fmt.Errorf("failed to write artifact %s: %w", filename, err)
		}

		fmt.Printf("✅ Decoded: %s\n", filename)
	}

	return nil
}

// generateSummary generates execution summary from GitHub Actions payload
func generateSummary(cmd *cobra.Command, args []string) error {
	payloadPath := args[0]

	// Read and parse payload
	data, err := os.ReadFile(payloadPath)
	if err != nil {
		return fmt.Errorf("failed to read payload file: %w", err)
	}

	var payload GitHubPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("failed to parse payload JSON: %w", err)
	}

	// Determine status and emoji
	var emoji, status string
	if payload.Success {
		emoji = "✅"
		status = "SUCCESS"
	} else {
		emoji = "❌"
		status = "FAILURE"
	}

	// Generate summary content
	summaryContent := fmt.Sprintf(`## %s External Test Results: %s

### 📍 Execution Details
| Property | Value |
| :--- | :--- |
| **Source** | %s |
| **Branch** | `+"`%s`"+` |
| **Commit** | `+"`%s`"+` |
| **Artifact Name** | %s |
`, emoji, status, payload.Source, payload.Branch, payload.Commit, payload.ArtifactName)

	// Write to GitHub Actions step summary if available
	summaryPath := os.Getenv("GITHUB_STEP_SUMMARY")
	if summaryPath != "" {
		file, err := os.OpenFile(summaryPath, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open GitHub summary file: %w", err)
		}
		defer file.Close()

		if _, err := file.WriteString(summaryContent); err != nil {
			return fmt.Errorf("failed to write to GitHub summary: %w", err)
		}
	} else {
		// If not in GitHub Actions, output to stdout
		fmt.Print(summaryContent)
	}

	return nil
}