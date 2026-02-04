#!/bin/bash
# RabbitMQ Initialization Script
# Runs after RabbitMQ container starts to configure vhost, users, exchanges, and queues

set -e

# Wait for RabbitMQ to be ready
echo "Waiting for RabbitMQ to be ready..."
rabbitmq-diagnostics -q ping
echo "RabbitMQ is ready!"

# Create vhost
rabbitmqctl add_vhost brainnav_vhost || true

# Set permissions for default user (guest) - remove if needed
rabbitmqctl set_permissions -p brainnav_vhost brainnav ".*" ".*" ".*" || true

# Create exchange for segmentation tasks
rabbitmq-plugins enable rabbitmq_management || true
rabbitmq-plugins enable rabbitmq_prometheus || true

# Configure segmentation exchange and queues
rabbitmqctl eval '
  rabbit_exchange:declare(segmentation, direct, true, false, false, []),
  rabbit_queue:declare(segmentation_tasks, true, false, false, [], []),
  rabbit_binding:add(segmentation_tasks, exchange, segmentation, <<"segmentation">>, []),
  rabbit_queue:declare(segmentation_dlx_queue, true, false, false, [], []),
  rabbit_binding:add(segmentation_dlx_queue, exchange, <<"segmentation_dlx">>, <<"segmentation_dlx">>, [])
' -p brainnav_vhost 2>/dev/null || echo "Exchange and queues setup skipped (may already exist)"

echo "RabbitMQ configuration completed!"
