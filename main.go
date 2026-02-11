package main

import (
	"encoding/json"
	"fmt"
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

var reportFile string

func main() {
	var rootCmd = &cobra.Command{
		Use:   "home-ci-reporter",
		Short: "E2E test report generator",
		Long:  "Generates YAML test reports for e2e tests with atomic operations ensuring valid YAML at all times",
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

	stepCmd.Flags().StringVarP(&reportFile, "file", "f", "", "Report file path (required)")
	stepCmd.MarkFlagRequired("file")

	finalizeCmd.Flags().StringVarP(&reportFile, "file", "f", "", "Report file path (required)")
	finalizeCmd.MarkFlagRequired("file")

	rootCmd.AddCommand(initCmd, stepCmd, finalizeCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func initReport(cmd *cobra.Command, args []string) error {
	reportPath := args[0]
	projectName := ""
	if len(args) > 1 {
		projectName = args[1]
	}

	// Create directory if needed
	if err := os.MkdirAll(filepath.Dir(reportPath), 0755); err != nil {
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

	return writeReport(reportPath, report)
}

func addStep(cmd *cobra.Command, args []string) error {
	phase := args[0]
	status := args[1]
	message := args[2]

	report, err := readReport(reportFile)
	if err != nil {
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
func debugPrint(report TestReport) {
	data, _ := json.MarshalIndent(report, "", "  ")
	fmt.Fprintf(os.Stderr, "DEBUG: %s\n", data)
}