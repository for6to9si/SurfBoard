package xrayclient

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/xtls/xray-core/app/observatory/command"
)

// ListVPNStatuses подключается к Xray и возвращает статус всех Outbound-соединений
func ListVPNStatuses() string {
	// Подключение
	conn, err := grpc.NewClient("dns:///127.0.0.1:10085",
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("Xray: ошибка подключения: %v", err)
		return "⚠️ Не удалось подключиться к Xray"
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("Xray: ошибка при закрытии соединения: %v", err)
		}
	}()

	client := pb.NewObservatoryServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.GetOutboundStatus(ctx, &pb.GetOutboundStatusRequest{})
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
