provider "hcloud" {
  token         = var.hcloud_token
  poll_interval = "2s"
}

provider "helm" {
  repository_config_path = "${path.module}/.helm/repositories.yaml"

  kubernetes = {
    config_path = "${path.module}/.helm/kubeconfig"
  }
}
