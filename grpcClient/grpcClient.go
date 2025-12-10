package grpcClient

import (
	"SurfBoard/benchmarkMode"
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"SurfBoard/conf"

	"github.com/xtls/xray-core/app/router"
	routerService "github.com/xtls/xray-core/app/router/command"
	creflect "github.com/xtls/xray-core/common/reflect"
	cserial "github.com/xtls/xray-core/common/serial"
	"google.golang.org/protobuf/proto"

	pbcf "github.com/xtls/xray-core/infra/conf"
	"github.com/xtls/xray-core/infra/conf/serial"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pbObserv "github.com/xtls/xray-core/app/observatory/command"
	pbRoute "github.com/xtls/xray-core/app/router/command"
)

// GRpcClient представляет клиента для взаимодействия с Xray gRPC-сервером
type GRpcClient struct {
	address string
	conn    *grpc.ClientConn
}

// NewGRpcClient создаёт новый экземпляр GRpcClient с установленным gRPC-соединением
func NewGRpcClient(grpcConfig conf.Grpc) (*GRpcClient, error) {
	address := fmt.Sprintf("dns:///%s:%d", grpcConfig.Target.IP, grpcConfig.Target.Port)
	fmt.Printf("Создан GRpcClient с IP: %s, Port: %d\n", grpcConfig.Target.IP, grpcConfig.Target.Port)

	conn, err := grpc.NewClient(address,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("ошибка подключения к Xray: %v", err)
	}

	return &GRpcClient{
		address: address,
		conn:    conn,
	}, nil
}

// Close закрывает gRPC-соединение
func (c *GRpcClient) Close() error {
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}

// ListVPNStatuses возвращает статус всех Outbound-соединений
func (c *GRpcClient) ListVPNStatuses() (string, []string) {

	tags := []string{}
	if c.conn == nil {
		log.Printf("Xray: соединение не инициализировано")
		return "⚠️ Соединение не инициализировано", tags
	}

	client := pbObserv.NewObservatoryServiceClient(c.conn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.GetOutboundStatus(ctx, &pbObserv.GetOutboundStatusRequest{})
	if err != nil {
		log.Printf("Xray: ошибка запроса: %v", err)
		return "⚠️ Не удалось получить статус VPN", tags
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

	for i, s := range statuses {
		icon := "✅"
		if !s.Alive {
			icon = "❌"
		}
		sb.WriteString(fmt.Sprintf("%d. %s %s — %d мс\n", i, icon, s.OutboundTag, s.Delay))
		tags = append(tags, s.OutboundTag)
	}

	sb.WriteString(fmt.Sprintf("\nПринудительно установить VPN: отправьте боту в Telegram команду `set n`, где n - номер VPN."))
	return sb.String(), tags
}

// GetCurrentVPN возвращает текущий активный VPN
func (c *GRpcClient) GetCurrentVPN() string {
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

func OverrideBalancerTarget(c *GRpcClient, balancerTag, target string) string {
	if c.conn == nil {
		log.Printf("Xray: соединение не инициализировано")
		return "⚠️ Соединение не инициализировано"
	}

	client := pbRoute.NewRoutingServiceClient(c.conn)

	// Ограничение по времени
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Запрос
	req := &pbRoute.OverrideBalancerTargetRequest{
		BalancerTag: balancerTag,
		Target:      target,
	}

	resp, err := client.OverrideBalancerTarget(ctx, req)
	if err != nil {
		log.Printf("Ошибка при вызове OverrideBalancerTarget: %v", err)
		return "⚠️ Ошибка при вызове OverrideBalancerTarget"
	}

	return fmt.Sprintf("✅ Балансер %q переопределён на %q\nОтвет: %+v", balancerTag, target, resp)
}

// ListVPNStatuses возвращает статус всех Outbound-соединений
func (c *GRpcClient) AddDomainsConf(Env conf.Env, FileTmpRoutingBalancers string, domainlist []string) (*pbcf.Config, []string) {

	fullpath := filepath.Join(Env.XrayLocationConfdir, FileTmpRoutingBalancers)

	ensureXrayAssetLocation(Env)

	output, results := benchmarkMode.ModifyDomainsJson(fullpath, domainlist)

	conf, err := serial.DecodeJSONConfig(bytes.NewBuffer(output))

	if err != nil {
		results = append(results, fmt.Sprintf("не удалось декодировать json %s: %s", output, err))
	}

	return conf, results
}

func (c *GRpcClient) AddDomainsRulesBuild(conf *pbcf.Config) (config *router.Config, results []string, benchmarkTime time.Duration) {

	var rcs *pbcf.RouterConfig

	// Если сама конфигурация == nil
	if conf == nil {
		results = append(results, "Ошибка: конфигурация пуста (conf == nil)")
		return nil, results, 0
	}

	// Если RouterConfig не задан
	if conf.RouterConfig == nil {
		results = append(results, "Ошибка: конфигурация маршрутизатора отсутствует (conf.RouterConfig == nil)")
		return nil, results, 0
	}

	rcs = conf.RouterConfig

	start := time.Now()
	config, err := rcs.Build()
	benchmarkTime = time.Since(start)
	log.Printf("benchmarkTime route Build(): %s\n", benchmarkTime)
	if err != nil {
		results = append(results, fmt.Sprintf("failed to build conf: %s", err))
	}
	return
}

func (c *GRpcClient) AddDomainsAddRule(config *router.Config) (results []string) {

	start := time.Now()
	tmsg := cserial.ToTypedMessage(config)
	log.Printf("cserial.ToTypedMessage: %s\n", time.Since(start))
	if tmsg == nil {
		results = append(results, fmt.Sprintf("failed to format config to TypedMessage."))
	}

	ra := &routerService.AddRuleRequest{
		Config:       tmsg,
		ShouldAppend: false,
	}

	client := routerService.NewRoutingServiceClient(c.conn)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	start = time.Now()
	resp, err := client.AddRule(ctx, ra)
	log.Printf("AddRule: %s\n", time.Since(start))
	if err != nil {
		results = append(results, fmt.Sprintf("failed to perform AddRule: %s", err))
		results = append(results, fmt.Sprintf("🔄 Xray нужен рестарт | 👻 API призрачно испустил дух"))
	}

	results = append(results, showJSONResponse(resp))

	return results
}

func ensureXrayAssetLocation(Env conf.Env) {
	const envASSET = "XRAY_LOCATION_ASSET"
	if os.Getenv(envASSET) == "" {
		// Можно убедиться, что путь существует (по желанию)
		defaultPath := Env.XrayLocationAsset
		if _, err := os.Stat(defaultPath); err == nil {
			_ = os.Setenv(envASSET, defaultPath)
		}
	}
}

func showJSONResponse(m proto.Message) string {
	if isNil(m) {
		return "ничего"
	}
	if j, ok := creflect.MarshalToJson(m, true); ok {
		return fmt.Sprintf(j)
	} else {
		return fmt.Sprintln(os.Stdout, "%v\n", m)
	}
}

func isNil(i interface{}) bool {
	vi := reflect.ValueOf(i)
	if vi.Kind() == reflect.Ptr {
		return vi.IsNil()
	}
	return i == nil
}
