package xrayclient

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"SurfBoard/conf"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pbObserv "github.com/xtls/xray-core/app/observatory/command"
	pbRoute "github.com/xtls/xray-core/app/router/command"
)

// XrayClient представляет клиента для взаимодействия с Xray gRPC-сервером
type XrayClient struct {
	address string
	conn    *grpc.ClientConn
}

// NewXrayClient создаёт новый экземпляр XrayClient с установленным gRPC-соединением
func NewXrayClient(grpcConfig conf.Grpc) (*XrayClient, error) {
	address := fmt.Sprintf("dns:///%s:%d", grpcConfig.Target.IP, grpcConfig.Target.Port)
	fmt.Printf("Создан XrayClient с IP: %s, Port: %d\n", grpcConfig.Target.IP, grpcConfig.Target.Port)

	conn, err := grpc.NewClient(address,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("ошибка подключения к Xray: %v", err)
	}

	return &XrayClient{
		address: address,
		conn:    conn,
	}, nil
}

// Close закрывает gRPC-соединение
func (c *XrayClient) Close() error {
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}

// ListVPNStatuses возвращает статус всех Outbound-соединений
func (c *XrayClient) ListVPNStatuses() string {
	if c.conn == nil {
		log.Printf("Xray: соединение не инициализировано")
		return "⚠️ Соединение не инициализировано"
	}

	client := pbObserv.NewObservatoryServiceClient(c.conn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.GetOutboundStatus(ctx, &pbObserv.GetOutboundStatusRequest{})
	if err != nil {
		log.Printf("Xray: ошибка запроса: %v", err)
		return "⚠️ Не удалось получить статус VPN"
	}

	statuses := resp.GetStatus().GetStatus()

	sort.Slice(statuses, func(i, j int) bool {
		if statuses[i].Alive != statuses[j].Alive {
			return statuses[i].Alive // живые вверх
		}
		return statuses[i].Delay < statuses[j].Delay
	})

	var sb strings.Builder
	sb.WriteString("📡 Список VPN и их статус:\n\n")

	for _, s := range statuses {
		icon := "✅"
		if !s.Alive {
			icon = "❌"
		}
		sb.WriteString(fmt.Sprintf("%s %s — %d мс\n", icon, s.OutboundTag, s.Delay))
	}

	return sb.String()
}

// GetCurrentVPN возвращает текущий активный VPN
func (c *XrayClient) GetCurrentVPN() string {
	if c.conn == nil {
		log.Printf("Xray: соединение не инициализировано")
		return "⚠️ Соединение не инициализировано"
	}

	client := pbRoute.NewRoutingServiceClient(c.conn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.GetBalancerInfo(ctx, &pbRoute.GetBalancerInfoRequest{
		Tag: "bestVPN",
	})
	if err != nil {
		log.Printf("Xray: ошибка запроса: %v", err)
		return "⚠️ Ошибка при запросе"
	}

	balancer := resp.GetBalancer()
	if balancer == nil {
		return "⚠️ Балансер не найден"
	}

	// 1. Если есть override.target — используем его
	if override := balancer.GetOverride(); override != nil && override.GetTarget() != "" {
		return override.GetTarget()
	}

	// 2. Иначе берём первый из principleTarget.tag
	if tags := balancer.GetPrincipleTarget().GetTag(); len(tags) > 0 {
		return tags[0]
	}

	return "⚠️ Нет доступных VPN"
}
