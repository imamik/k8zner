# Image Builder

The `image` package provides functionality to build custom Talos Linux disk images on Hetzner Cloud.

## How it works

1.  **Provisioning:** A temporary server is created using a standard Linux image (e.g., Debian 12) in rescue mode or with a bootstrap script.
2.  **Installation:** The Talos installer is downloaded and executed to write the Talos image to the server's disk.
3.  **Snapshot:** A snapshot of the server is taken. This snapshot contains the bootable Talos image.
4.  **Cleanup:** The temporary server is deleted.

## Usage

```go
builder := image.NewBuilder(client, communicator)
snapshotID, err := builder.Build(ctx, "talos-v1.8.0")
```

## State Machine

The build process follows a linear state machine:

`Init -> CreateServer -> WaitForIP -> ProvisionTalos -> CreateSnapshot -> Cleanup`

If any step fails, `Cleanup` is ensured via `defer`.
