output "address" {
  description = "the A record play.phons.dev points at, grey cloud"
  value       = aws_eip.game.public_ip
}

output "instance_id" {
  description = "what aws ssm start-session and the start/stop commands target"
  value       = aws_instance.game.id
}
