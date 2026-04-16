package rabbitmq

import (
	"encoding/json"
	"ethclient_service/config"
	"ethclient_service/logger"
	"fmt"
	"sync"

	"github.com/streadway/amqp"
)

// Client RabbitMQ客户端
type Client struct {
	config    config.RabbitMQConfig
	conn      *amqp.Connection
	channel   *amqp.Channel
	stopChan  chan struct{}
	isRunning bool
	mu        sync.RWMutex
}

// TransferEventMessage 转账事件消息
type TransferEventMessage struct {
	ChainID         string `json:"chain_id"`
	ChainName       string `json:"chain_name"`
	BlockNumber     int64  `json:"block_number"`
	BlockHash       string `json:"block_hash"`
	TransactionHash string `json:"transaction_hash"`
	LogIndex        int    `json:"log_index"`
	From            string `json:"from"`
	To              string `json:"to"`
	Value           string `json:"value"`
	Timestamp       int64  `json:"timestamp"`
}

// NewClient 创建RabbitMQ客户端
func NewClient(cfg config.RabbitMQConfig) (*Client, error) {
	client := &Client{
		config:   cfg,
		stopChan: make(chan struct{}),
	}

	if err := client.connect(); err != nil {
		return nil, err
	}

	if err := client.declare(); err != nil {
		return nil, err
	}

	return client, nil
}

// connect 建立连接
func (c *Client) connect() error {
	url := fmt.Sprintf("amqp://%s:%s@%s:%d/",
		c.config.User,
		c.config.Password,
		c.config.Host,
		c.config.Port,
	)

	conn, err := amqp.Dial(url)
	if err != nil {
		return fmt.Errorf("连接RabbitMQ失败: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return fmt.Errorf("创建Channel失败: %w", err)
	}

	c.conn = conn
	c.channel = ch
	c.isRunning = true

	logger.Log.Info("✅ RabbitMQ连接成功")
	return nil
}

// declare 声明交换机和队列
func (c *Client) declare() error {
	// 声明交换机
	err := c.channel.ExchangeDeclare(
		c.config.Exchange, // name
		"topic",           // type
		true,              // durable
		false,             // auto-deleted
		false,             // internal
		false,             // no-wait
		nil,               // arguments
	)
	if err != nil {
		return fmt.Errorf("声明交换机失败: %w", err)
	}

	// 声明队列
	_, err = c.channel.QueueDeclare(
		c.config.Queue, // name
		true,           // durable
		false,          // delete when unused
		false,          // exclusive
		false,          // no-wait
		nil,            // arguments
	)
	if err != nil {
		return fmt.Errorf("声明队列失败: %w", err)
	}

	// 绑定队列到交换机
	err = c.channel.QueueBind(
		c.config.Queue,      // queue name
		c.config.RoutingKey, // routing key
		c.config.Exchange,   // exchange
		false,               // no-wait
		nil,                 // arguments
	)
	if err != nil {
		return fmt.Errorf("绑定队列失败: %w", err)
	}

	logger.Log.Infof("✅ RabbitMQ交换机/队列声明成功: exchange=%s, queue=%s",
		c.config.Exchange, c.config.Queue)
	return nil
}

// PublishEvent 发布事件到MQ
func (c *Client) PublishEvent(msg *TransferEventMessage) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.isRunning {
		return fmt.Errorf("RabbitMQ客户端未运行")
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %w", err)
	}

	err = c.channel.Publish(
		c.config.Exchange,   // exchange
		c.config.RoutingKey, // routing key
		false,               // mandatory
		false,               // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent, // 持久化消息
		},
	)
	if err != nil {
		return fmt.Errorf("发布消息失败: %w", err)
	}

	return nil
}

// Consume 消费消息
func (c *Client) Consume(handler func(*TransferEventMessage) error) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.isRunning {
		return fmt.Errorf("RabbitMQ客户端未运行")
	}

	msgs, err := c.channel.Consume(
		c.config.Queue, // queue
		"",             // consumer
		false,          // auto-ack (手动确认)
		false,          // exclusive
		false,          // no-local
		false,          // no-wait
		nil,            // args
	)
	if err != nil {
		return fmt.Errorf("开始消费失败: %w", err)
	}

	go func() {
		for {
			select {
			case msg, ok := <-msgs:
				if !ok {
					logger.Log.Warn("RabbitMQ消费通道关闭")
					return
				}

				var event TransferEventMessage
				if err := json.Unmarshal(msg.Body, &event); err != nil {
					logger.Log.Errorf("解析消息失败: %v", err)
					msg.Nack(false, false) // 丢弃消息
					continue
				}

				if err := handler(&event); err != nil {
					logger.Log.Errorf("处理消息失败: %v", err)
					msg.Nack(false, true) // 重新入队
					continue
				}

				msg.Ack(false) // 确认消息

			case <-c.stopChan:
				return
			}
		}
	}()

	logger.Log.Info("✅ RabbitMQ开始消费消息")
	return nil
}

// Close 关闭连接
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.isRunning {
		return
	}

	close(c.stopChan)
	c.isRunning = false

	if c.channel != nil {
		c.channel.Close()
	}
	if c.conn != nil {
		c.conn.Close()
	}

	logger.Log.Info("🛑 RabbitMQ连接已关闭")
}

// IsRunning 检查是否运行中
func (c *Client) IsRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isRunning
}
