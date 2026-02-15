package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// secretEntry represents a single secret for display.
type secretEntry struct {
	Category string `json:"category"`
	Name     string `json:"name"`
	Value    string `json:"value"`
}

// Secrets retrieves and displays cluster secrets.
func Secrets(ctx context.Context, configPath string, jsonOutput bool) error {
	cfg, err := loadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
		return fmt.Errorf("kubeconfig not found. Run 'k8zner apply' first to create the cluster")
	}

	kubecfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}
	kubecfg.Timeout = 10 * time.Second

	k8sClient, err := client.New(kubecfg, client.Options{})
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	entries := collectSecrets(ctx, k8sClient)

	if jsonOutput {
		b, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to encode JSON: %w", err)
		}
		fmt.Println(string(b))
		return nil
	}

	printSecretsStyled(cfg.ClusterName, entries)
	return nil
}

func collectSecrets(ctx context.Context, k8sClient client.Client) []secretEntry {
	var entries []secretEntry

	// Files
	entries = append(entries, secretEntry{
		Category: "Files",
		Name:     "kubeconfig",
		Value:    kubeconfigPath,
	})
	if _, err := os.Stat(talosConfigPath); err == nil {
		entries = append(entries, secretEntry{
			Category: "Files",
			Name:     "talosconfig",
			Value:    talosConfigPath,
		})
	}

	// ArgoCD
	secret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: "argocd", Name: "argocd-initial-admin-secret"}, secret); err == nil {
		if password, ok := secret.Data["password"]; ok {
			entries = append(entries, secretEntry{
				Category: "ArgoCD",
				Name:     "admin password",
				Value:    string(password),
			})
		}
	}

	// Grafana
	grafanaSecret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: "monitoring", Name: "kube-prometheus-stack-grafana"}, grafanaSecret); err == nil {
		if username, ok := grafanaSecret.Data["admin-user"]; ok {
			entries = append(entries, secretEntry{
				Category: "Grafana",
				Name:     "admin username",
				Value:    string(username),
			})
		}
		if password, ok := grafanaSecret.Data["admin-password"]; ok {
			entries = append(entries, secretEntry{
				Category: "Grafana",
				Name:     "admin password",
				Value:    string(password),
			})
		}
	}

	return entries
}

func printSecretsStyled(clusterName string, entries []secretEntry) {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#f9fafb"))
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#3b82f6"))
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))

	fmt.Println()
	fmt.Println(titleStyle.Render(fmt.Sprintf("  k8zner secrets: %s", clusterName)))
	fmt.Println(dimStyle.Render("  " + strings.Repeat("=", 30)))
	fmt.Println()

	currentCategory := ""
	for _, entry := range entries {
		if entry.Category != currentCategory {
			if currentCategory != "" {
				fmt.Println()
			}
			fmt.Println(sectionStyle.Render("  " + entry.Category))
			fmt.Println(dimStyle.Render("  " + strings.Repeat("-", 35)))
			currentCategory = entry.Category
		}
		fmt.Printf("  %s  %s\n", nameStyle.Render(fmt.Sprintf("%-18s", entry.Name)), valueStyle.Render(entry.Value))
	}

	if len(entries) == 0 {
		fmt.Println(dimStyle.Render("  No secrets found. Is the cluster running?"))
	}

	fmt.Println()
}
