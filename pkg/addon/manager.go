package addon

// Manager handles cluster addons.
type Manager struct {
    // In the future, this will manage CCM, CSI, CNI.
}

func NewManager() *Manager {
    return &Manager{}
}

func (m *Manager) GetManifests() ([]string, error) {
    // Placeholder for manifests
    return []string{}, nil
}
