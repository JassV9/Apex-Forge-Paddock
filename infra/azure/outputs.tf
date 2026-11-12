output "url" {
  description = "Where the demo answers."
  value       = "https://${azurerm_container_app.demo.ingress[0].fqdn}"
}

output "resource_group" {
  description = "Everything the demo created is in this group; deleting it removes all of it."
  value       = azurerm_resource_group.demo.name
}
