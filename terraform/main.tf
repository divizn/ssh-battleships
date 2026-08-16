locals {
  # opening hours, shared with the website
  hours = jsondecode(file("${path.module}/../hours.json"))
  days  = join(",", [for d in local.hours.days : upper(substr(d, 0, 3))])
}

provider "aws" {
  region = var.region

  default_tags {
    tags = {
      Project   = var.name
      ManagedBy = "terraform"
    }
  }
}

data "aws_vpc" "default" {
  default = true
}

data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
}

data "aws_ssm_parameter" "al2023" {
  name = "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64"
}

resource "aws_security_group" "game" {
  name        = var.name
  description = "ssh battleships, open to players"
  vpc_id      = data.aws_vpc.default.id
}

# the game itself answers on 22, so this is the front door, not administration
resource "aws_vpc_security_group_ingress_rule" "players_v4" {
  security_group_id = aws_security_group.game.id
  cidr_ipv4         = "0.0.0.0/0"
  from_port         = 22
  to_port           = 22
  ip_protocol       = "tcp"
}

resource "aws_vpc_security_group_ingress_rule" "players_v6" {
  security_group_id = aws_security_group.game.id
  cidr_ipv6         = "::/0"
  from_port         = 22
  to_port           = 22
  ip_protocol       = "tcp"
}

resource "aws_vpc_security_group_egress_rule" "out" {
  security_group_id = aws_security_group.game.id
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "-1"
}

data "aws_iam_policy_document" "ec2_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "instance" {
  name               = "${var.name}-instance"
  assume_role_policy = data.aws_iam_policy_document.ec2_assume.json
}

# session manager replaces an administrative sshd, which is what frees port 22
resource "aws_iam_role_policy_attachment" "ssm" {
  role       = aws_iam_role.instance.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

data "aws_iam_policy_document" "secrets" {
  statement {
    actions   = ["ssm:GetParameter"]
    resources = ["arn:aws:ssm:${var.region}:*:parameter/${var.name}/*"]
  }
  statement {
    actions   = ["kms:Decrypt"]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "kms:ViaService"
      values   = ["ssm.${var.region}.amazonaws.com"]
    }
  }
}

resource "aws_iam_role_policy" "secrets" {
  name   = "${var.name}-secrets"
  role   = aws_iam_role.instance.id
  policy = data.aws_iam_policy_document.secrets.json
}

resource "aws_iam_instance_profile" "instance" {
  name = "${var.name}-instance"
  role = aws_iam_role.instance.name
}

resource "aws_instance" "game" {
  ami                    = data.aws_ssm_parameter.al2023.value
  instance_type          = "t4g.nano"
  subnet_id              = data.aws_subnets.default.ids[0]
  vpc_security_group_ids = [aws_security_group.game.id]
  iam_instance_profile   = aws_iam_instance_profile.instance.name

  user_data = templatefile("${path.module}/user_data.sh", {
    name   = var.name
    repo   = var.repo
    region = var.region
  })

  # user_data only runs on a fresh instance, so editing the script has to rebuild the box or it
  # buys a stop/start and changes nothing
  user_data_replace_on_change = true

  root_block_device {
    volume_size = 8
    volume_type = "gp3"
    encrypted   = true
  }

  metadata_options {
    http_tokens = "required"
  }

  tags = { Name = var.name }

  # a fresh amazon linux release should not silently replace a running game
  lifecycle {
    ignore_changes = [ami]
  }
}

# the instance is stopped outside opening hours, and a stopped instance loses its automatic
# address, so play.phons.dev needs one that survives
resource "aws_eip" "game" {
  instance = aws_instance.game.id
  domain   = "vpc"
  tags     = { Name = var.name }
}

data "aws_iam_policy_document" "scheduler_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["scheduler.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "scheduler" {
  name               = "${var.name}-scheduler"
  assume_role_policy = data.aws_iam_policy_document.scheduler_assume.json
}

data "aws_iam_policy_document" "power" {
  statement {
    actions   = ["ec2:StartInstances", "ec2:StopInstances"]
    resources = ["arn:aws:ec2:${var.region}:*:instance/${aws_instance.game.id}"]
  }
}

resource "aws_iam_role_policy" "power" {
  name   = "${var.name}-power"
  role   = aws_iam_role.scheduler.id
  policy = data.aws_iam_policy_document.power.json
}

resource "aws_scheduler_schedule" "open" {
  name                         = "${var.name}-open"
  schedule_expression          = "cron(0 ${local.hours.open} ? * ${local.days} *)"
  schedule_expression_timezone = local.hours.timezone

  flexible_time_window {
    mode = "OFF"
  }

  target {
    arn      = "arn:aws:scheduler:::aws-sdk:ec2:startInstances"
    role_arn = aws_iam_role.scheduler.arn
    input    = jsonencode({ InstanceIds = [aws_instance.game.id] })
  }
}

resource "aws_scheduler_schedule" "close" {
  name                         = "${var.name}-close"
  schedule_expression          = "cron(0 ${local.hours.close} ? * ${local.days} *)"
  schedule_expression_timezone = local.hours.timezone

  flexible_time_window {
    mode = "OFF"
  }

  target {
    arn      = "arn:aws:scheduler:::aws-sdk:ec2:stopInstances"
    role_arn = aws_iam_role.scheduler.arn
    input    = jsonencode({ InstanceIds = [aws_instance.game.id] })
  }
}
