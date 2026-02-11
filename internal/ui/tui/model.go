package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/ui/benchmarks"
)

// BootstrapPhase represents a CLI bootstrap phase for display.
type BootstrapPhase struct {
	Name   string
	Done   bool
	Active bool
	Err    error
}

// Model is the Bubble Tea model for the TUI dashboard.
type Model struct {
	// Cluster info
	ClusterName string
	Region      string

	// Bootstrap phases (apply command)
	BootstrapPhases []BootstrapPhase
	BootstrapDone   bool

	// CRD-sourced state
	ClusterPhase   k8znerv1alpha1.ClusterPhase
	ProvPhase      k8znerv1alpha1.ProvisioningPhase
	Infrastructure k8znerv1alpha1.InfrastructureStatus
	ControlPlanes  k8znerv1alpha1.NodeGroupStatus
	Workers        k8znerv1alpha1.NodeGroupStatus
	Addons         map[string]k8znerv1alpha1.AddonStatus
	PhaseHistory   []k8znerv1alpha1.PhaseRecord
	LastErrors     []k8znerv1alpha1.ErrorRecord
	LastReconcile  string

	// ETA
	EstimatedRemaining time.Duration
	StartTime          time.Time

	// UI state
	Width  int
	Height int
	Err    error
	Done   bool

	// Mode
	Mode string // "apply", "doctor"
}

// NewApplyModel creates a model for the apply command TUI.
func NewApplyModel(clusterName, region string) Model {
	return Model{
		ClusterName: clusterName,
		Region:      region,
		StartTime:   time.Now(),
		Mode:        "apply",
		BootstrapPhases: []BootstrapPhase{
			{Name: "Talos Image"},
			{Name: "Infrastructure"},
			{Name: "Control Plane"},
			{Name: "Bootstrap"},
			{Name: "Operator"},
			{Name: "CRD"},
		},
	}
}

// NewDoctorModel creates a model for the doctor command TUI.
func NewDoctorModel(clusterName string) Model {
	return Model{
		ClusterName: clusterName,
		StartTime:   time.Now(),
		Mode:        "doctor",
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return tickCmd()
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height

	case BootstrapPhaseMsg:
		m.updateBootstrapPhase(msg)
		if msg.Err != nil {
			m.Err = msg.Err
			return m, tea.Quit
		}

	case CRDStatusMsg:
		m.updateCRDStatus(msg)
		if m.ClusterPhase == k8znerv1alpha1.ClusterPhaseRunning && m.Mode == "apply" {
			m.Done = true
			return m, tea.Quit
		}

	case TickMsg:
		m.updateETA()
		return m, tickCmd()

	case ErrMsg:
		m.Err = msg.Err
		return m, tea.Quit

	case DoneMsg:
		m.Done = true
		return m, tea.Quit
	}

	return m, nil
}

func (m *Model) updateBootstrapPhase(msg BootstrapPhaseMsg) {
	phaseMap := map[string]int{
		"image":          0,
		"infrastructure": 1,
		"compute":        2,
		"bootstrap":      3,
		"operator":       4,
		"crd":            5,
	}

	idx, ok := phaseMap[msg.Phase]
	if !ok {
		return
	}

	// Mark previous phases as done
	for i := 0; i < idx; i++ {
		m.BootstrapPhases[i].Done = true
		m.BootstrapPhases[i].Active = false
	}

	if msg.Done {
		m.BootstrapPhases[idx].Done = true
		m.BootstrapPhases[idx].Active = false
		if idx == len(m.BootstrapPhases)-1 {
			m.BootstrapDone = true
		}
	} else {
		m.BootstrapPhases[idx].Active = true
	}

	if msg.Err != nil {
		m.BootstrapPhases[idx].Err = msg.Err
	}
}

func (m *Model) updateCRDStatus(msg CRDStatusMsg) {
	m.ClusterPhase = msg.ClusterPhase
	m.ProvPhase = msg.ProvPhase
	m.Infrastructure = msg.Infrastructure
	m.ControlPlanes = msg.ControlPlanes
	m.Workers = msg.Workers
	m.Addons = msg.Addons
	m.PhaseHistory = msg.PhaseHistory
	m.LastErrors = msg.LastErrors
	m.LastReconcile = msg.LastReconcile
}

func (m *Model) updateETA() {
	if string(m.ProvPhase) == "" || m.ProvPhase == k8znerv1alpha1.PhaseComplete {
		m.EstimatedRemaining = 0
		return
	}

	var phaseElapsed time.Duration
	for _, rec := range m.PhaseHistory {
		if rec.EndedAt == nil && string(rec.Phase) == string(m.ProvPhase) {
			phaseElapsed = time.Since(rec.StartedAt.Time)
			break
		}
	}

	m.EstimatedRemaining = benchmarks.EstimateRemaining(string(m.ProvPhase), phaseElapsed, m.PhaseHistory)
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(_ time.Time) tea.Msg {
		return TickMsg{}
	})
}

// View implements tea.Model.
func (m Model) View() string {
	return renderView(m)
}
