package hcloud

import (
	"context"
	"fmt"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// EnsureCertificate ensures that a certificate exists with the given specifications.
func (c *RealClient) EnsureCertificate(ctx context.Context, name, certificate, privateKey string, labels map[string]string) (*hcloud.Certificate, error) {
	return reconcileResource(ctx, name, ReconcileFuncs[hcloud.Certificate]{
		Get: func(ctx context.Context, name string) (*hcloud.Certificate, error) {
			cert, _, err := c.client.Certificate.Get(ctx, name)
			return cert, err
		},
		Create: func(ctx context.Context) (*hcloud.Certificate, error) {
			opts := hcloud.CertificateCreateOpts{
				Name:        name,
				Certificate: certificate,
				PrivateKey:  privateKey,
				Labels:      labels,
				Type:        hcloud.CertificateTypeUploaded,
			}

			// Create returns (*Certificate, *Response, error) for Uploaded certificates.
			res, _, err := c.client.Certificate.Create(ctx, opts)
			if err != nil {
				return nil, fmt.Errorf("failed to create certificate: %w", err)
			}
			return res, nil
		},
		NeedsUpdate: nil,
		Update:      nil,
	})
}

// GetCertificate returns the certificate with the given name.
func (c *RealClient) GetCertificate(ctx context.Context, name string) (*hcloud.Certificate, error) {
	cert, _, err := c.client.Certificate.Get(ctx, name)
	return cert, err
}

// DeleteCertificate deletes the certificate with the given name.
func (c *RealClient) DeleteCertificate(ctx context.Context, name string) error {
	return deleteResource(ctx, name, DeleteFuncs[hcloud.Certificate]{
		Get: func(ctx context.Context, name string) (*hcloud.Certificate, error) {
			cert, _, err := c.client.Certificate.Get(ctx, name)
			return cert, err
		},
		Delete: func(ctx context.Context, cert *hcloud.Certificate) error {
			_, err := c.client.Certificate.Delete(ctx, cert)
			return err
		},
	}, c.getGenericTimeouts())
}
