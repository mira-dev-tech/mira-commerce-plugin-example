// Package exampleloyalty é o plugin DIDÁTICO de referência do modelo de
// extensibilidade — um programa de fidelidade mínimo que exercita as três
// superfícies que um plugin real usa:
//
//  1. Hook síncrono que AJUSTA resultado    → checkout.quote_adjust (desconto)
//  2. Hook síncrono que BLOQUEIA fluxo      → order.pre_confirm (teto de pedido)
//  3. Evento assíncrono (at-least-once)     → order.confirmed (acúmulo de pontos)
//
// Regras do protocolo que este arquivo demonstra na prática:
//
//   - Plugin importa SOMENTE o módulo público commerce-ext — nunca
//     mira-commerce-core/internal/... (anti-pattern, ver ARCHITECTURE §10).
//   - Hooks são síncronos, com timeout de 300ms e panic-safe no host: se o
//     handler estourar, o host degrada (deny com CodePluginFailure) — por isso
//     handlers devem ser rápidos e sem I/O bloqueante.
//   - Handlers de evento DEVEM ser idempotentes: a entrega é at-least-once e o
//     mesmo fato de negócio pode chegar em MAIS de um evento (com IDs
//     distintos — ex.: submit emite order.confirmed pelo workflow engine E pelo
//     caminho direto). Deduplique pela CHAVE DE NEGÓCIO (aqui, order_id); em
//     produção, tabela com UNIQUE(order_id) no schema ext_<plugin> — ADR 0004.
//   - Config vem do Runtime (manifest/host) com fallback a env — nunca
//     hardcode de valores de negócio.
//
// Guia passo-a-passo: docs/plugin-example.md no mira-commerce-core.
// Este repositório é o TEMPLATE standalone — veja o README para os dois modos
// de carga (in-process no core ou binário externo go-plugin via cmd/).
package exampleloyalty

import (
	"context"
	"os"
	"strconv"
	"sync"

	commerceext "github.com/mira-dev-tech/commerce-ext"
)

// Config do plugin — resolvida em Init a partir de Runtime.Config com
// fallback a variáveis de ambiente EXAMPLE_LOYALTY_*.
const (
	defaultMinSubtotal = 300 // subtotal mínimo (R$) para ganhar desconto
	defaultDiscountPct = 5   // percentual de desconto
	defaultOrderCap    = 0   // teto de valor por pedido; 0 = desligado
)

// Plugin implementa commerceext.Plugin. Uma instância por processo.
type Plugin struct {
	logger commerceext.Logger

	// Config imutável após Init — sem lock.
	minSubtotal float32
	discountPct float32
	orderCap    float32

	// Estado do handler de evento. ATENÇÃO: em memória SÓ porque este plugin é
	// didático — estado real de plugin vai para Postgres no schema
	// ext_example_loyalty (ADR 0004), nunca em memória de processo.
	mu        sync.Mutex
	processed map[string]struct{} // order IDs já creditados (idempotência por chave de negócio)
	points    int64
}

// New devolve a instância do plugin (chamado pelo loader do core).
func New() *Plugin {
	return &Plugin{processed: make(map[string]struct{})}
}

// Meta identifica o plugin. CompatibleCore é validado contra a linha do
// protocolo commerce-ext (semver) na carga do manifest.
func (p *Plugin) Meta() commerceext.Meta {
	return commerceext.Meta{
		ID:             "example-loyalty",
		Version:        "1.0.0",
		CompatibleCore: "^0.2.0",
		Description:    "Plugin didático — fidelidade: desconto no quote, teto de pedido e pontos por evento",
	}
}

// Init recebe o Runtime (config, secrets, logger, eventos) antes de Register.
// É o único momento de ler config — depois disso os handlers só consomem.
func (p *Plugin) Init(_ context.Context, rt *commerceext.Runtime) error {
	p.logger = commerceext.NopLogger{}
	var cfg map[string]any
	if rt != nil {
		if rt.Logger != nil {
			p.logger = rt.Logger
		}
		cfg = rt.Config
	}
	p.minSubtotal = configFloat(cfg, "min_subtotal", "EXAMPLE_LOYALTY_MIN_SUBTOTAL", defaultMinSubtotal)
	p.discountPct = configFloat(cfg, "discount_pct", "EXAMPLE_LOYALTY_DISCOUNT_PCT", defaultDiscountPct)
	p.orderCap = configFloat(cfg, "max_order_total", "EXAMPLE_LOYALTY_MAX_ORDER_TOTAL", defaultOrderCap)
	p.logger.Info("example-loyalty init",
		"min_subtotal", p.minSubtotal, "discount_pct", p.discountPct, "order_cap", p.orderCap)
	return nil
}

// Register declara hooks e eventos. Prioridade: menor executa primeiro na
// cadeia do hook; 400 deixa espaço para plugins de cliente antes (100–300) e
// depois (500+, onde ficam os plugins de referência do core).
func (p *Plugin) Register(reg *commerceext.Registry) error {
	reg.OnCheckoutQuoteAdjust(400, p.onQuoteAdjust)
	reg.OnOrderPreConfirm(400, p.onPreConfirm)
	reg.OnEvent(commerceext.EventOrderConfirmed, p.onOrderConfirmed)
	return nil
}

// Shutdown libera recursos (conexões, workers). Aqui não há nada a fechar.
func (p *Plugin) Shutdown(context.Context) error {
	return nil
}

// onQuoteAdjust — hook checkout.quote_adjust, roda em buildOrder ANTES de
// persistir o pedido. Semântica da cadeia (ver RunCheckoutQuoteAdjust):
// Discount ACUMULA entre plugins; TotalAmount > 0 SOBRESCREVE o total corrente.
// Devolver Outcome deny bloqueia o checkout inteiro com o código informado.
func (p *Plugin) onQuoteAdjust(_ context.Context, in commerceext.QuoteAdjustInput) commerceext.QuoteAdjustResult {
	if in.Subtotal < p.minSubtotal {
		// Sem ajuste: Allow com TotalAmount zero mantém o total corrente.
		return commerceext.QuoteAdjustResult{Outcome: commerceext.Allowed()}
	}
	discount := in.Subtotal * p.discountPct / 100
	p.logger.Info("loyalty discount applied",
		"tenant_id", in.TenantID, "subtotal", in.Subtotal, "discount", discount)
	return commerceext.QuoteAdjustResult{
		TotalAmount: in.Subtotal - discount,
		Discount:    discount,
		Outcome:     commerceext.Allowed(),
	}
}

// onPreConfirm — hook order.pre_confirm, roda no submit do pedido. Demonstra
// bloqueio com código ESTÁVEL: o front e o suporte dependem desse código, então
// trate-o como contrato (não renomear sem ADR).
func (p *Plugin) onPreConfirm(_ context.Context, order commerceext.OrderView) commerceext.Outcome {
	if p.orderCap > 0 && order.TotalAmount > p.orderCap {
		p.logger.Warn("order blocked by loyalty cap",
			"order_id", order.ID, "total", order.TotalAmount, "cap", p.orderCap)
		return commerceext.Denied("LOYALTY_ORDER_CAP", "Pedido acima do limite do programa de fidelidade.")
	}
	return commerceext.Allowed()
}

// onOrderConfirmed — evento order.confirmed (assíncrono, at-least-once).
// Idempotência pela CHAVE DE NEGÓCIO: deduplicamos por order_id, não por
// Event.ID — o mesmo pedido pode gerar mais de um evento com IDs distintos
// (workflow engine + caminho direto no submit). Erro retornado aqui só é
// logado — eventos nunca bloqueiam o fluxo que os originou.
func (p *Plugin) onOrderConfirmed(_ context.Context, e commerceext.Event) error {
	orderID, _ := e.Data["order_id"].(string)
	if orderID == "" {
		p.logger.Warn("order.confirmed sem order_id — ignorando", "event_id", e.ID)
		return nil
	}
	total := eventFloat(e.Data, "total_amount")
	earned := int64(total) // 1 ponto por R$ 1 — regra ilustrativa

	p.mu.Lock()
	defer p.mu.Unlock()
	if _, done := p.processed[orderID]; done {
		p.logger.Info("order already credited — skipping", "order_id", orderID, "event_id", e.ID)
		return nil
	}
	p.processed[orderID] = struct{}{}
	p.points += earned
	p.logger.Info("loyalty points accrued",
		"order_id", e.Data["order_id"], "earned", earned, "total_points", p.points)
	return nil
}

// Points expõe o acumulado para testes e introspecção.
func (p *Plugin) Points() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.points
}

// configFloat resolve uma chave de config: Runtime.Config → env → default.
func configFloat(cfg map[string]any, key, envKey string, def float32) float32 {
	if cfg != nil {
		switch v := cfg[key].(type) {
		case float64:
			return float32(v)
		case float32:
			return v
		case int:
			return float32(v)
		case string:
			if f, err := strconv.ParseFloat(v, 32); err == nil {
				return float32(f)
			}
		}
	}
	if raw := os.Getenv(envKey); raw != "" {
		if f, err := strconv.ParseFloat(raw, 32); err == nil && f >= 0 {
			return float32(f)
		}
	}
	return def
}

// eventFloat lê um número de Event.Data (JSON decodifica números como float64).
func eventFloat(data map[string]any, key string) float64 {
	if data == nil {
		return 0
	}
	switch v := data[key].(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	}
	return 0
}
