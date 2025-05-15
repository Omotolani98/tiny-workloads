package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss" // Use lipgloss for styles
)

// ConfigSpec represents the application specifications provided as input.
type ConfigSpec struct {
	AppName         string
	ExpectedLoad    int    // Example: Number of expected requests per second
	DataSize        int    // Example: Size of data to be processed in MB
	NetworkTraffic  int    // Example: Expected network bandwidth in Mbps
	ImportanceLevel string // Example: "high", "medium", "low"
}

// ComputeSpec represents the decided compute resources.
type ComputeSpec struct {
	CPU    float64 // in cores
	Memory string  // in Mi or Gi
}

// NetworkSpec represents the decided network resources.
type NetworkSpec struct {
	Bandwidth string // in Mbps
	Ports     []int
}

// StorageSpec represents the decided storage resources.
type StorageSpec struct {
	Capacity string // in Gi
	Class    string // e.g., "standard", "premium"
}

// TimedResult struct to hold the outcome and duration of each decision function.
type TimedResult struct {
	Name        string
	ComputeSpec ComputeSpec
	NetworkSpec NetworkSpec
	StorageSpec StorageSpec
	Error       error
	Duration    time.Duration
}

// Messages for Bubble Tea
type ProcessCompleteMsg map[string]TimedResult
type ProcessErrorMsg error
type TickMsg time.Time // Message for the processing spinner

// Model for the Bubble Tea application
type model struct {
	// Wizard state
	inputs     []textinput.Model
	list       list.Model // For ImportanceLevel
	focused    int        // Which input is focused
	inputState inputState // Enum for current input type (text, list)
	quitting   bool       // Flag to indicate if we are quitting

	// ConfigSpec values collected
	config ConfigSpec

	// Process state
	processing   bool
	spinner      string // Spinner character for processing
	spinnerFrame int    // Current frame of the spinner
	result       map[string]TimedResult
	err          error

	// Output state
	k8sManifestPath string
	output          string // Glamour-rendered output

	// Terminal size
	width  int
	height int
}

type inputState int

const (
	inputStateText inputState = iota
	inputStateList
	inputStateProcessing
	inputStateDone
)

// List item for ImportanceLevel
type item string

func (i item) FilterValue() string { return "" }

type itemDelegate struct{}

func (d itemDelegate) Height() int                               { return 1 }
func (d itemDelegate) Spacing() int                              { return 0 }
func (d itemDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil } // Corrected signature
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(item)
	if !ok {
		return
	}

	str := fmt.Sprintf("%d. %s", index+1, string(i))

	// Corrected logic to use the style's Render method directly
	style := itemStyle
	if index == m.Index() {
		style = selectedItemStyle
		str = "> " + str // Add the indicator for the selected item
	}

	fmt.Fprint(w, style.Render(str)) // Call the style's Render method
}

// Define consistent column widths
const (
	labelWidth = 24
	inputWidth = 30
)

var (
	// Use lipgloss for styles
	itemStyle         = lipgloss.NewStyle().PaddingLeft(2)
	selectedItemStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).PaddingLeft(2) // Green
	listTitleStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")).Bold(true)     // Yellow

	// Improved styles for layout with consistent column widths
	labelStyle         = lipgloss.NewStyle().Width(labelWidth).Align(lipgloss.Right).PaddingRight(1)
	inputRowStyle      = lipgloss.NewStyle().PaddingBottom(1)
	textInputViewStyle = lipgloss.NewStyle().Width(inputWidth)

	// Style for prompt and placeholder
	focusedPromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00"))
	blurredPromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))

	// Styles for input fields
	focusedInputStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#00FF00"))
	blurredInputStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#666666"))

	// Spinner characters
	spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
)

// Initial state of the application
func initialModel() model {
	inputs := make([]textinput.Model, 4) // AppName, ExpectedLoad, DataSize, NetworkTraffic

	// AppName
	inputs[0] = textinput.New()
	inputs[0].Placeholder = "e.g. my-web-app"
	inputs[0].Focus()
	inputs[0].PromptStyle = focusedPromptStyle
	inputs[0].PlaceholderStyle = blurredPromptStyle
	inputs[0].CharLimit = 30
	inputs[0].Prompt = "> "
	inputs[0].TextStyle = focusedInputStyle    // Apply style to the text itself
	inputs[0].PromptStyle = focusedPromptStyle // Apply style to the prompt

	// ExpectedLoad
	inputs[1] = textinput.New()
	inputs[1].Placeholder = "e.g. 500"
	inputs[1].CharLimit = 6
	inputs[1].Prompt = "> "
	inputs[1].Validate = func(s string) error {
		_, err := strconv.Atoi(s)
		if s != "" && err != nil {
			return fmt.Errorf("must be a number")
		}
		return nil
	}
	inputs[1].TextStyle = blurredInputStyle
	inputs[1].PromptStyle = blurredPromptStyle

	// DataSize
	inputs[2] = textinput.New()
	inputs[2].Placeholder = "e.g. 100"
	inputs[2].CharLimit = 6
	inputs[2].Prompt = "> "
	inputs[2].Validate = func(s string) error {
		_, err := strconv.Atoi(s)
		if s != "" && err != nil {
			return fmt.Errorf("must be a number")
		}
		return nil
	}
	inputs[2].TextStyle = blurredInputStyle
	inputs[2].PromptStyle = blurredPromptStyle

	// NetworkTraffic
	inputs[3] = textinput.New()
	inputs[3].Placeholder = "e.g. 75"
	inputs[3].CharLimit = 6
	inputs[3].Prompt = "> "
	inputs[3].Validate = func(s string) error {
		_, err := strconv.Atoi(s)
		if s != "" && err != nil {
			return fmt.Errorf("must be a number")
		}
		return nil
	}
	inputs[3].TextStyle = blurredInputStyle
	inputs[3].PromptStyle = blurredPromptStyle

	// ImportanceLevel list
	items := []list.Item{
		item("high"),
		item("medium"),
		item("low"),
	}
	listModel := list.New(items, itemDelegate{}, 20, 10)
	listModel.Title = "Select Importance Level"
	listModel.SetShowStatusBar(false)
	listModel.SetFilteringEnabled(false)
	listModel.Styles.Title = listTitleStyle

	return model{
		inputs:     inputs,
		list:       listModel,
		focused:    0,
		inputState: inputStateText,
		spinner:    spinnerFrames[0],
	}
}

// Bubble Tea Init function
func (m model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, tickCmd()) // Start blinking and spinner ticking
}

// Command to send a TickMsg periodically for the spinner
func tickCmd() tea.Cmd {
	return tea.Every(time.Millisecond*100, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

// Bubble Tea Update function
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			return m, tea.Quit

		case tea.KeyEnter:
			if m.inputState == inputStateText {
				if m.focused < len(m.inputs)-1 {
					// Move to the next text input
					m.inputs[m.focused].Blur()
					m.inputs[m.focused].PromptStyle = blurredPromptStyle
					m.inputs[m.focused].TextStyle = blurredInputStyle
					m.focused++
					m.inputs[m.focused].Focus()
					m.inputs[m.focused].PromptStyle = focusedPromptStyle
					m.inputs[m.focused].TextStyle = focusedInputStyle
				} else {
					// Finished text inputs, move to list
					m.inputs[m.focused].Blur()
					m.inputs[m.focused].PromptStyle = blurredPromptStyle
					m.inputs[m.focused].TextStyle = blurredInputStyle
					m.inputState = inputStateList
				}
			} else if m.inputState == inputStateList {
				// Selected item from the list, trigger processing
				selectedItem, ok := m.list.SelectedItem().(item)
				if !ok {
					m.err = fmt.Errorf("failed to get selected importance level")
					m.inputState = inputStateDone
					return m, nil
				}
				m.config.ImportanceLevel = string(selectedItem)

				// Collect all inputs
				m.config.AppName = m.inputs[0].Value()
				load, err := strconv.Atoi(m.inputs[1].Value())
				if err != nil {
					m.err = fmt.Errorf("invalid expected load: %v", err)
					m.inputState = inputStateDone
					return m, nil
				}
				m.config.ExpectedLoad = load

				dataSize, err := strconv.Atoi(m.inputs[2].Value())
				if err != nil {
					m.err = fmt.Errorf("invalid data size: %v", err)
					m.inputState = inputStateDone
					return m, nil
				}
				m.config.DataSize = dataSize

				networkTraffic, err := strconv.Atoi(m.inputs[3].Value())
				if err != nil {
					m.err = fmt.Errorf("invalid network traffic: %v", err)
					m.inputState = inputStateDone
					return m, nil
				}
				m.config.NetworkTraffic = networkTraffic

				// Start the background processing
				m.processing = true
				m.inputState = inputStateProcessing
				return m, tea.Batch(processResourceAllocation(m.config), tickCmd()) // Start processing and continue spinner
			}

		case tea.KeyShiftTab, tea.KeyCtrlP:
			// Move to the previous input
			if m.inputState == inputStateText {
				m.inputs[m.focused].Blur()
				m.inputs[m.focused].PromptStyle = blurredPromptStyle
				m.inputs[m.focused].TextStyle = blurredInputStyle
				m.focused = max(0, m.focused-1)
				m.inputs[m.focused].Focus()
				m.inputs[m.focused].PromptStyle = focusedPromptStyle
				m.inputs[m.focused].TextStyle = focusedInputStyle
			} else if m.inputState == inputStateList {
				// Move from list back to last text input
				m.inputState = inputStateText
				m.focused = len(m.inputs) - 1
				m.inputs[m.focused].Focus()
				m.inputs[m.focused].PromptStyle = focusedPromptStyle
				m.inputs[m.focused].TextStyle = focusedInputStyle
			}

		case tea.KeyTab, tea.KeyCtrlN:
			// Move to the next input
			if m.inputState == inputStateText {
				if m.focused < len(m.inputs)-1 {
					m.inputs[m.focused].Blur()
					m.inputs[m.focused].PromptStyle = blurredPromptStyle
					m.inputs[m.focused].TextStyle = blurredInputStyle
					m.focused++
					m.inputs[m.focused].Focus()
					m.inputs[m.focused].PromptStyle = focusedPromptStyle
					m.inputs[m.focused].TextStyle = focusedInputStyle
				} else {
					// Finished text inputs, move to list
					m.inputs[m.focused].Blur()
					m.inputs[m.focused].PromptStyle = blurredPromptStyle
					m.inputs[m.focused].TextStyle = blurredInputStyle
					m.inputState = inputStateList
				}
			}
		}

	case ProcessCompleteMsg:
		m.processing = false
		m.result = msg
		m.inputState = inputStateDone

		// Generate and write Kubernetes manifest
		err := m.generateAndWriteManifest()
		if err != nil {
			m.err = fmt.Errorf("failed to generate/write manifest: %v", err)
		}

		// Generate output string with Glamour
		m.output = m.generateOutput()

		return m, tea.Quit // Quit after processing is done and output is generated

	case ProcessErrorMsg:
		m.processing = false
		m.err = msg
		m.inputState = inputStateDone

		// Generate output string with Glamour
		m.output = m.generateOutput()

		return m, tea.Quit // Quit on error

	case TickMsg: // Update spinner frame
		if m.processing {
			m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
			m.spinner = spinnerFrames[m.spinnerFrame]
			return m, tickCmd() // Continue ticking
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Handle window resizing for the list
		if m.inputState == inputStateList {
			m.list.SetWidth(msg.Width)
			m.list.SetHeight(m.height - 10)
		}
	}

	// Update the focused text input or the list
	if m.inputState == inputStateText {
		m.inputs[m.focused], cmd = m.inputs[m.focused].Update(msg)
	} else if m.inputState == inputStateList {
		m.list, cmd = m.list.Update(msg)
	}

	return m, cmd
}

// Bubble Tea View function
func (m model) View() string {
	if m.quitting {
		return "Exiting...\n"
	}

	if m.inputState == inputStateProcessing {
		return fmt.Sprintf("%s Processing resource allocation for %s...\n", m.spinner, m.config.AppName)
	}

	if m.inputState == inputStateDone {
		if m.err != nil {
			// Render the error using Glamour
			errorOutput, renderErr := glamour.Render(fmt.Sprintf("# Error\n\n```\n%v\n```", m.err), "dark")
			if renderErr != nil {
				return fmt.Sprintf("Error rendering error message: %v\nOriginal Error:\n%v\n", renderErr, m.err)
			}
			return errorOutput
		}
		// Render the final output using Glamour
		out, err := glamour.Render(m.output, "dark") // Use "dark" or "light" style
		if err != nil {
			return fmt.Sprintf("Error rendering output: %v\nOutput:\n%s\n", err, m.output)
		}
		return out
	}

	// Render the input wizard
	var b strings.Builder

	b.WriteString("Enter application specifications:\n\n")

	// Use a consistent grid layout for labels and inputs
	inputLabels := []string{"App Name:", "Expected Load (RPS):", "Data Size (MB):", "Network Traffic (Mbps):"}
	for i := range m.inputs {
		// Create a consistent row with aligned labels and inputs
		row := lipgloss.JoinHorizontal(lipgloss.Top,
			labelStyle.Render(inputLabels[i]),
			textInputViewStyle.Render(m.inputs[i].View()),
		)
		b.WriteString(inputRowStyle.Render(row) + "\n")
	}

	if m.inputState == inputStateList {
		b.WriteString("\n")
		b.WriteString(m.list.View())
	} else {
		listPlaceholder := labelStyle.Render("Select Importance Level:")
		b.WriteString(inputRowStyle.Render(listPlaceholder))
	}

	b.WriteString("\nPress Enter to continue, Tab/Shift+Tab to navigate, Ctrl+C to quit.\n")

	return b.String()
}

// Helper function to run resource allocation in a goroutine
func processResourceAllocation(config ConfigSpec) tea.Cmd {
	return func() tea.Msg {
		timedResults, err := collectTimedResourceSpecs(&config)
		if err != nil {
			return ProcessErrorMsg(err)
		}
		return ProcessCompleteMsg(timedResults)
	}
}

// The resource allocation logic
func collectTimedResourceSpecs(config *ConfigSpec) (map[string]TimedResult, error) {
	resultChan := make(chan TimedResult, 3)
	go config.decideCompute(resultChan)
	go config.decideNetwork(resultChan)
	go config.decideStorage(resultChan)

	timedResults := make(map[string]TimedResult)
	for i := 0; i < 3; i++ {
		result := <-resultChan
		if result.Error != nil {
			return nil, fmt.Errorf("error in %s decision: %v", result.Name, result.Error)
		}
		timedResults[result.Name] = result
	}
	return timedResults, nil
}

func (config *ConfigSpec) decideCompute(resultChan chan<- TimedResult) {
	fmt.Printf("Starting compute decision for %s...\n", config.AppName)
	startTime := time.Now()
	time.Sleep(time.Millisecond * 200) // Simulate some computation

	cpu := float64(config.ExpectedLoad) / 150 // Adjusted: Even tinier compute
	memory := "256Mi"                         // Default tinier memory

	if config.ExpectedLoad > 300 {
		cpu += 0.75
		memory = "512Mi"
	}
	if config.ImportanceLevel == "high" {
		cpu += 0.25
		memory = "1Gi"
	}
	if config.DataSize > 50 {
		memory = fmt.Sprintf("%dMi", 256+(config.DataSize/4))
	}

	duration := time.Since(startTime)
	resultChan <- TimedResult{
		Name:        "compute",
		ComputeSpec: ComputeSpec{CPU: cpu, Memory: memory},
		Duration:    duration,
		Error:       nil,
	}
	// fmt.Printf("Compute decision finished for %s.\n", config.AppName)
}

func (config *ConfigSpec) decideNetwork(resultChan chan<- TimedResult) {
	fmt.Printf("Starting network decision for %s...\n", config.AppName)
	startTime := time.Now()
	time.Sleep(time.Millisecond * 150) // Simulate some computation

	bandwidth := "50Mbps"
	ports := []int{8080}

	if config.NetworkTraffic > 25 {
		bandwidth = "200Mbps"
	}
	if config.ImportanceLevel == "high" {
		ports = append(ports, 443)
	}

	duration := time.Since(startTime)
	resultChan <- TimedResult{
		Name:        "network",
		NetworkSpec: NetworkSpec{Bandwidth: bandwidth, Ports: ports},
		Duration:    duration,
		Error:       nil,
	}
	// fmt.Printf("Network decision finished for %s.\n", config.AppName)
}

func (config *ConfigSpec) decideStorage(resultChan chan<- TimedResult) {
	fmt.Printf("Starting storage decision for %s...\n", config.AppName)
	startTime := time.Now()
	time.Sleep(time.Millisecond * 100) // Simulate some computation

	capacity := "5Gi"
	class := "standard"

	if config.DataSize > 250 {
		capacity = fmt.Sprintf("%dGi", 5+(config.DataSize/100))
		class = "premium"
	}
	if config.ImportanceLevel == "high" {
		capacity = "20Gi"
	}

	duration := time.Since(startTime)
	resultChan <- TimedResult{
		Name:        "storage",
		StorageSpec: StorageSpec{Capacity: capacity, Class: class},
		Duration:    duration,
		Error:       nil,
	}
	// fmt.Printf("Storage decision finished for %s.\n", config.AppName)
}

// Generates the Kubernetes manifest string
func generateKubernetesManifest(appName string, timedResults map[string]TimedResult, ports []int) string {
	var portString string
	for _, port := range ports {
		portString += fmt.Sprintf(`
          - containerPort: %d
            protocol: TCP
        `, port)
	}

	computeResult := timedResults["compute"]
	networkResult := timedResults["network"]
	storageResult := timedResults["storage"]

	manifest := fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s-deployment
spec:
  replicas: 1
  selector:
    matchLabels:
      app: %s
  template:
    metadata:
      labels:
        app: %s
    spec:
      containers:
      - name: %s-container
        image: your-app-image:latest # Replace with your actual image
        resources:
          requests:
            cpu: "%.2f"
            memory: "%s"
          limits:
            cpu: "%.2f"
            memory: "%s"
        ports:
%s
        env:
          - name: NETWORK_BANDWIDTH
            value: "%s"
          - name: STORAGE_CAPACITY
            value: "%s"
          - name: STORAGE_CLASS
            value: "%s"
`, appName, appName, appName, appName,
		computeResult.ComputeSpec.CPU*0.8, computeResult.ComputeSpec.Memory,
		computeResult.ComputeSpec.CPU, computeResult.ComputeSpec.Memory,
		portString,
		networkResult.NetworkSpec.Bandwidth,
		storageResult.StorageSpec.Capacity,
		storageResult.StorageSpec.Class,
	)
	return manifest
}

// Generates and writes the Kubernetes manifest file
func (m *model) generateAndWriteManifest() error {
	if m.result == nil {
		return fmt.Errorf("no results available to generate manifest")
	}

	k8sManifest := generateKubernetesManifest(m.config.AppName, m.result, m.result["network"].NetworkSpec.Ports)
	outputPath := filepath.Join("k8s", fmt.Sprintf("%s-deployment.yaml", m.config.AppName))

	// Create the "k8s" directory if it doesn't exist
	err := os.MkdirAll("k8s", 0755)
	if err != nil {
		return fmt.Errorf("error creating k8s directory: %v", err)
	}

	// Write the manifest to the specified file
	err = os.WriteFile(outputPath, []byte(k8sManifest), 0644)
	if err != nil {
		return fmt.Errorf("error writing Kubernetes manifest: %v", err)
	}

	m.k8sManifestPath = outputPath
	return nil
}

// Generates the final output string in Markdown format for Glamour
func (m *model) generateOutput() string {
	if m.err != nil {
		// Error rendering is handled in View()
		return ""
	}

	if m.result == nil {
		return "# No Results" // Should not happen if err is nil
	}

	var sb strings.Builder

	sb.WriteString("# Resource Allocation Decision\n\n")

	computeResult := m.result["compute"]
	networkResult := m.result["network"]
	storageResult := m.result["storage"]

	sb.WriteString("## Decided Resources\n\n")
	sb.WriteString(fmt.Sprintf("- **Compute:** CPU=%.2f cores, Memory=%s (took %s)\n",
		computeResult.ComputeSpec.CPU,
		computeResult.ComputeSpec.Memory,
		computeResult.Duration,
	))
	sb.WriteString(fmt.Sprintf("- **Network:** Bandwidth=%s, Ports=%v (took %s)\n",
		networkResult.NetworkSpec.Bandwidth,
		networkResult.NetworkSpec.Ports,
		networkResult.Duration,
	))
	sb.WriteString(fmt.Sprintf("- **Storage:** Capacity=%s, Class=%s (took %s)\n",
		storageResult.StorageSpec.Capacity,
		storageResult.StorageSpec.Class,
		storageResult.Duration,
	))

	if m.k8sManifestPath != "" {
		sb.WriteString(fmt.Sprintf("\n## File Generated\n\nDeployment file generated within `%s`\n", m.k8sManifestPath))
	}

	return sb.String()
}

func main() {
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Printf("AlloCAT error: %v\n", err)
		os.Exit(1)
	}
}

// Helper function for max
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
