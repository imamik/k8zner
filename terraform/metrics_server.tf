locals {
  metrics_server_schedule_on_control_plane = coalesce(
    var.metrics_server_schedule_on_control_plane,
    local.worker_sum == 0,
    false
  )
  metrics_server_node_sum = (
    local.metrics_server_schedule_on_control_plane ?
    local.control_plane_sum :
    local.worker_sum > 0 ? local.worker_sum : local.cluster_autoscaler_max_sum
  )
  metrics_server_replicas = coalesce(
    var.metrics_server_replicas,
    local.metrics_server_node_sum > 1 ? 2 : 1
  )
}

data "helm_template" "metrics_server" {
  count = var.metrics_server_enabled ? 1 : 0

  name      = "metrics-server"
  namespace = "kube-system"

  repository   = var.metrics_server_helm_repository
  chart        = var.metrics_server_helm_chart
  version      = var.metrics_server_helm_version
  kube_version = var.kubernetes_version

  values = [
    yamlencode({
      replicas = local.metrics_server_replicas
      podDisruptionBudget = {
        enabled        = true
        minAvailable   = null
        maxUnavailable = 1
      }
      topologySpreadConstraints = [
        {
          topologyKey       = "kubernetes.io/hostname"
          maxSkew           = 1
          whenUnsatisfiable = "DoNotSchedule"
          labelSelector = {
            matchLabels = {
              "app.kubernetes.io/instance" = "metrics-server"
              "app.kubernetes.io/name"     = "metrics-server"
            }
          }
          matchLabelKeys = ["pod-template-hash"]
        },
        {
          topologyKey       = "topology.kubernetes.io/zone"
          maxSkew           = 1
          whenUnsatisfiable = "ScheduleAnyway"
          labelSelector = {
            matchLabels = {
              "app.kubernetes.io/instance" = "metrics-server"
              "app.kubernetes.io/name"     = "metrics-server"
            }
          }
          matchLabelKeys = ["pod-template-hash"]
        }
      ]
      nodeSelector = local.metrics_server_schedule_on_control_plane ? {
        "node-role.kubernetes.io/control-plane" = ""
      } : {}
      tolerations = local.metrics_server_schedule_on_control_plane ? [
        {
          key      = "node-role.kubernetes.io/control-plane"
          effect   = "NoSchedule"
          operator = "Exists"
        }
      ] : []
    }),
    yamlencode(var.metrics_server_helm_values)
  ]
}

locals {
  metrics_server_manifest = var.metrics_server_enabled ? {
    name     = "metrics-server"
    contents = data.helm_template.metrics_server[0].manifest
  } : null
}
