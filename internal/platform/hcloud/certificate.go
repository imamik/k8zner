package hcloud

import (
	"context"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// EnsureCertificate ensures that a certificate exists with the given specifications.
func (c *RealClient) EnsureCertificate(ctx context.Context, name, certificate, privateKey string, labels map[string]string) (*hcloud.Certificate, error) {
	return (&EnsureOperation[*hcloud.Certificate, hcloud.CertificateCreateOpts, any]{
		Name:         name,
		ResourceType: "certificate",
		Get:          c.client.Certificate.Get,
		Create:       simpleCreate(c.client.Certificate.Create),
		CreateOptsMapper: func() hcloud.CertificateCreateOpts {
			return hcloud.CertificateCreateOpts{
				Name:        name,
				Certificate: certificate,
				PrivateKey:  privateKey,
				Labels:      labels,
				Type:        hcloud.CertificateTypeUploaded,
			}
		},
	}).Execute(ctx, c)
}

// GetCertificate returns the certificate with the given name.
func (c *RealClient) GetCertificate(ctx context.Context, name string) (*hcloud.Certificate, error) {
	cert, _, err := c.client.Certificate.Get(ctx, name)
	return cert, err
}

// DeleteCertificate deletes the certificate with the given name.
func (c *RealClient) DeleteCertificate(ctx context.Context, name string) error {
	return (&DeleteOperation[*hcloud.Certificate]{
		Name:         name,
		ResourceType: "certificate",
		Get:          c.client.Certificate.Get,
		Delete:       c.client.Certificate.Delete,
	}).Execute(ctx, c)
}
