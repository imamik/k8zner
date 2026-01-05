locals {
  rdns_cluster_domain_pattern = "/{{\\s*cluster-domain\\s*}}/"
  rdns_cluster_name_pattern   = "/{{\\s*cluster-name\\s*}}/"
  rdns_hostname_pattern       = "/{{\\s*hostname\\s*}}/"
  rdns_id_pattern             = "/{{\\s*id\\s*}}/"
  rdns_ip_labels_pattern      = "/{{\\s*ip-labels\\s*}}/"
  rdns_ip_type_pattern        = "/{{\\s*ip-type\\s*}}/"
  rdns_pool_pattern           = "/{{\\s*pool\\s*}}/"
  rdns_role_pattern           = "/{{\\s*role\\s*}}/"

  cluster_rdns_ipv4 = var.cluster_rdns_ipv4 != null ? var.cluster_rdns_ipv4 : var.cluster_rdns
  cluster_rdns_ipv6 = var.cluster_rdns_ipv6 != null ? var.cluster_rdns_ipv6 : var.cluster_rdns

  ingress_load_balancer_rdns_ipv4 = (
    var.ingress_load_balancer_rdns_ipv4 != null ? var.ingress_load_balancer_rdns_ipv4 :
    var.ingress_load_balancer_rdns != null ? var.ingress_load_balancer_rdns :
    local.cluster_rdns_ipv4
  )
  ingress_load_balancer_rdns_ipv6 = (
    var.ingress_load_balancer_rdns_ipv6 != null ? var.ingress_load_balancer_rdns_ipv6 :
    var.ingress_load_balancer_rdns != null ? var.ingress_load_balancer_rdns :
    local.cluster_rdns_ipv6
  )
}

resource "hcloud_rdns" "control_plane" {
  for_each = {
    for entry in flatten([
      for server in hcloud_server.control_plane : [
        for ip_type in concat(
          local.control_plane_nodepools_map[server.labels.nodepool].rdns_ipv4 != null ? ["ipv4"] : [],
          local.control_plane_nodepools_map[server.labels.nodepool].rdns_ipv6 != null ? ["ipv6"] : [],
          ) : {
          key = "${server.name}-${ip_type}"
          value = {
            ip_address = ip_type == "ipv4" ? server.ipv4_address : server.ipv6_address
            rdns = (
              ip_type == "ipv4" ?
              local.control_plane_nodepools_map[server.labels.nodepool].rdns_ipv4 :
              local.control_plane_nodepools_map[server.labels.nodepool].rdns_ipv6
            )
            hostname = server.name
            id       = server.id
            ip_labels = (
              ip_type == "ipv4" ?
              join(".", reverse(split(".", server.ipv4_address))) :
              join(".", reverse(flatten([
                for part in split(":", replace(
                  server.ipv6_address, "::", ":${join(":",
                    slice(
                      [0, 0, 0, 0, 0, 0, 0, 0],
                      0, 8 - length(compact(split(":", server.ipv6_address)))
                    )
                  )}:"
                )) : [for char in split("", format("%04s", part)) : char]
              ])))
            )
            ip_type = ip_type
            pool    = server.labels.nodepool
            role    = server.labels.role
          }
        }
      ]
    ]) : entry.key => entry.value
  }

  server_id  = each.value.id
  ip_address = each.value.ip_address
  dns_ptr = (
    replace(replace(replace(replace(replace(replace(replace(replace((each.value.rdns
      ), local.rdns_cluster_domain_pattern, var.cluster_domain
      ), local.rdns_cluster_name_pattern, var.cluster_name
      ), local.rdns_hostname_pattern, each.value.hostname
      ), local.rdns_id_pattern, each.value.id
      ), local.rdns_ip_labels_pattern, each.value.ip_labels
      ), local.rdns_ip_type_pattern, each.value.ip_type
      ), local.rdns_pool_pattern, each.value.pool
      ), local.rdns_role_pattern, each.value.role
    )
  )
}

resource "hcloud_rdns" "worker" {
  for_each = {
    for entry in flatten([
      for server in hcloud_server.worker : [
        for ip_type in concat(
          local.worker_nodepools_map[server.labels.nodepool].rdns_ipv4 != null ? ["ipv4"] : [],
          local.worker_nodepools_map[server.labels.nodepool].rdns_ipv6 != null ? ["ipv6"] : [],
          ) : {
          key = "${server.name}-${ip_type}"
          value = {
            ip_address = ip_type == "ipv4" ? server.ipv4_address : server.ipv6_address
            rdns = (
              ip_type == "ipv4" ?
              local.worker_nodepools_map[server.labels.nodepool].rdns_ipv4 :
              local.worker_nodepools_map[server.labels.nodepool].rdns_ipv6
            )
            hostname = server.name
            id       = server.id
            ip_labels = (
              ip_type == "ipv4" ?
              join(".", reverse(split(".", server.ipv4_address))) :
              join(".", reverse(flatten([
                for part in split(":", replace(
                  server.ipv6_address, "::", ":${join(":",
                    slice(
                      [0, 0, 0, 0, 0, 0, 0, 0],
                      0, 8 - length(compact(split(":", server.ipv6_address)))
                    )
                  )}:"
                )) : [for char in split("", format("%04s", part)) : char]
              ])))
            )
            ip_type = ip_type
            pool    = server.labels.nodepool
            role    = server.labels.role
          }
        }
      ]
    ]) : entry.key => entry.value
  }

  server_id  = each.value.id
  ip_address = each.value.ip_address
  dns_ptr = (
    replace(replace(replace(replace(replace(replace(replace(replace((each.value.rdns
      ), local.rdns_cluster_domain_pattern, var.cluster_domain
      ), local.rdns_cluster_name_pattern, var.cluster_name
      ), local.rdns_hostname_pattern, each.value.hostname
      ), local.rdns_id_pattern, each.value.id
      ), local.rdns_ip_labels_pattern, each.value.ip_labels
      ), local.rdns_ip_type_pattern, each.value.ip_type
      ), local.rdns_pool_pattern, each.value.pool
      ), local.rdns_role_pattern, each.value.role
    )
  )
}

resource "hcloud_rdns" "ingress" {
  for_each = {
    for entry in flatten([
      for lb in hcloud_load_balancer.ingress : [
        for ip_type in concat(
          local.ingress_load_balancer_rdns_ipv4 != null ? ["ipv4"] : [],
          local.ingress_load_balancer_rdns_ipv6 != null ? ["ipv6"] : [],
          ) : {
          key = "${lb.name}-${ip_type}"
          value = {
            ip_address = ip_type == "ipv4" ? lb.ipv4 : lb.ipv6
            rdns = (
              ip_type == "ipv4" ?
              local.ingress_load_balancer_rdns_ipv4 :
              local.ingress_load_balancer_rdns_ipv6
            )
            hostname = lb.name
            id       = lb.id
            ip_labels = (
              ip_type == "ipv4" ?
              join(".", reverse(split(".", lb.ipv4))) :
              join(".", reverse(flatten([
                for part in split(":", replace(
                  lb.ipv6, "::", ":${join(":",
                    slice(
                      [0, 0, 0, 0, 0, 0, 0, 0],
                      0, 8 - length(compact(split(":", lb.ipv6)))
                    )
                  )}:"
                )) : [for char in split("", format("%04s", part)) : char]
              ])))
            )
            ip_type = ip_type
            pool    = "ingress"
            role    = lb.labels.role
          }
        }
      ]
    ]) : entry.key => entry.value
  }

  load_balancer_id = each.value.id
  ip_address       = each.value.ip_address
  dns_ptr = (
    replace(replace(replace(replace(replace(replace(replace(replace((each.value.rdns
      ), local.rdns_cluster_domain_pattern, var.cluster_domain
      ), local.rdns_cluster_name_pattern, var.cluster_name
      ), local.rdns_hostname_pattern, each.value.hostname
      ), local.rdns_id_pattern, each.value.id
      ), local.rdns_ip_labels_pattern, each.value.ip_labels
      ), local.rdns_ip_type_pattern, each.value.ip_type
      ), local.rdns_pool_pattern, each.value.pool
      ), local.rdns_role_pattern, each.value.role
    )
  )
}

resource "hcloud_rdns" "ingress_pool" {
  for_each = {
    for entry in flatten([
      for lb in hcloud_load_balancer.ingress_pool : [
        for ip_type in concat(
          local.ingress_load_balancer_pools_map[lb.labels.pool].rdns_ipv4 != null ? ["ipv4"] : [],
          local.ingress_load_balancer_pools_map[lb.labels.pool].rdns_ipv6 != null ? ["ipv6"] : [],
          ) : {
          key = "${lb.name}-${ip_type}"
          value = {
            ip_address = ip_type == "ipv4" ? lb.ipv4 : lb.ipv6
            rdns = (
              ip_type == "ipv4" ?
              local.ingress_load_balancer_pools_map[lb.labels.pool].rdns_ipv4 :
              local.ingress_load_balancer_pools_map[lb.labels.pool].rdns_ipv6
            )
            hostname = lb.name
            id       = lb.id
            ip_labels = (
              ip_type == "ipv4" ?
              join(".", reverse(split(".", lb.ipv4))) :
              join(".", reverse(flatten([
                for part in split(":", replace(
                  lb.ipv6, "::", ":${join(":",
                    slice(
                      [0, 0, 0, 0, 0, 0, 0, 0],
                      0, 8 - length(compact(split(":", lb.ipv6)))
                    )
                  )}:"
                )) : [for char in split("", format("%04s", part)) : char]
              ])))
            )
            ip_type = ip_type
            pool    = lb.labels.pool
            role    = lb.labels.role
          }
        }
      ]
    ]) : entry.key => entry.value
  }

  load_balancer_id = each.value.id
  ip_address       = each.value.ip_address
  dns_ptr = (
    replace(replace(replace(replace(replace(replace(replace(replace((each.value.rdns
      ), local.rdns_cluster_domain_pattern, var.cluster_domain
      ), local.rdns_cluster_name_pattern, var.cluster_name
      ), local.rdns_hostname_pattern, each.value.hostname
      ), local.rdns_id_pattern, each.value.id
      ), local.rdns_ip_labels_pattern, each.value.ip_labels
      ), local.rdns_ip_type_pattern, each.value.ip_type
      ), local.rdns_pool_pattern, each.value.pool
      ), local.rdns_role_pattern, each.value.role
    )
  )
}

