package xrayclient

import (
	"SurfBoard/conf"
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pbObserv "github.com/xtls/xray-core/app/observatory/command"
	pbRoute "github.com/xtls/xray-core/app/router/command"
)

var address string

func Init(grpc conf.Grpc) {
	address = fmt.Sprintf("dns:///%s:%d", grpc.Target.IP, grpc.Target.Port)
	fmt.Printf("Using GRPC IP: %s, Port: %d\n", grpc.Target.IP, grpc.Target.Port)
}

// newGRPCClient создаёт новое gRPC-соединение и возвращает его вместе с функцией закрытия
func newGRPCClient() (*grpc.ClientConn, error) {
	conn, err := grpc.NewClient(address,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("ошибка подключения к Xray: %v", err)
	}
	return conn, nil
}

// ListVPNStatuses подключается к Xray и возвращает статус всех Outbound-соединений
func ListVPNStatuses() string {
	conn, err := newGRPCClient()
	if err != nil {
		log.Printf("Xray: %v", err)
		return "⚠️ Не удалось подключиться к Xray"
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("Xray: ошибка при закрытии соединения: %v", err)
		}
	}()

	client := pbObserv.NewObservatoryServiceClient(conn)

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
func GetCurrentVPN() string {
	conn, err := newGRPCClient()
	if err != nil {
		log.Printf("Xray: %v", err)
		return "⚠️ Не удалось подключиться"
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("Xray: ошибка при закрытии соединения: %v", err)
		}
	}()

	client := pbRoute.NewRoutingServiceClient(conn)

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
