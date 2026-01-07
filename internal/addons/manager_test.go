package addons

import (
	"context"
	"testing"

	"github.com/sak-d/hcloud-k8s/internal/config"
	"github.com/sak-d/hcloud-k8s/internal/k8s"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestEnsureHCloudSecret(t *testing.T) {
	fakeClientset := fake.NewSimpleClientset()
	kClient := &k8s.Client{
		Clientset: fakeClientset,
	}

	cfg := &config.Config{
		HCloudToken: "test-token",
	}

	mgr := &Manager{
		k8sClient: kClient,
		cfg:       cfg,
		networkID: 12345,
	}

	ctx := context.Background()
	err := mgr.ensureHCloudSecret(ctx)
	assert.NoError(t, err)

	// Verify creation
	secret, err := fakeClientset.CoreV1().Secrets("kube-system").Get(ctx, "hcloud", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, "test-token", string(secret.Data["token"]))
	assert.Equal(t, "12345", string(secret.Data["network"]))

	// Test Update
	mgr.cfg.HCloudToken = "new-token"
	err = mgr.ensureHCloudSecret(ctx)
	assert.NoError(t, err)

	secret, err = fakeClientset.CoreV1().Secrets("kube-system").Get(ctx, "hcloud", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, "new-token", string(secret.Data["token"]))
}

func TestEnsureCiliumIPSecSecret(t *testing.T) {
	fakeClientset := fake.NewSimpleClientset()
	kClient := &k8s.Client{
		Clientset: fakeClientset,
	}

	mgr := &Manager{
		k8sClient: kClient,
	}

	ctx := context.Background()
	err := mgr.ensureCiliumIPSecSecret(ctx)
	assert.NoError(t, err)

	secret, err := fakeClientset.CoreV1().Secrets("kube-system").Get(ctx, "cilium-ipsec-keys", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotEmpty(t, secret.Data["keys"])
	assert.Contains(t, string(secret.Data["keys"]), "rfc4106(gcm(aes))")
}
