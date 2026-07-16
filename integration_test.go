package exampleloyalty

// Teste de INTEGRAÇÃO LOCAL com o mini-host `plugintest`: exercita o plugin
// pela CADEIA (prioridades, timeout, semântica do quote_adjust e entrega
// at-least-once de eventos) sem precisar do core. É a etapa entre o teste
// unitário e o aceite em ambiente Mirá — rode antes de pedir slot em staging.

import (
	"context"
	"testing"
	"time"

	commerceext "github.com/mira-dev-tech/commerce-ext"
	"github.com/mira-dev-tech/commerce-ext/plugintest"
)

func newHost(t *testing.T, cfg map[string]any) (*plugintest.Host, *Plugin) {
	t.Helper()
	host := plugintest.NewHost()
	p := New()
	if err := host.Install(context.Background(), p, plugintest.WithConfig(cfg)); err != nil {
		t.Fatal(err)
	}
	return host, p
}

func TestIntegrationQuoteAdjustThroughChain(t *testing.T) {
	host, _ := newHost(t, nil)
	out := host.QuoteAdjust(context.Background(), commerceext.QuoteAdjustInput{Subtotal: 500})
	if !out.Allow || out.Discount != 25 || out.TotalAmount != 475 {
		t.Fatalf("expected 5%% pela cadeia (25/475), got %+v", out)
	}
	// Abaixo do mínimo a cadeia preserva o total.
	out = host.QuoteAdjust(context.Background(), commerceext.QuoteAdjustInput{Subtotal: 100})
	if !out.Allow || out.TotalAmount != 100 || out.Discount != 0 {
		t.Fatalf("expected pass-through, got %+v", out)
	}
}

func TestIntegrationPreConfirmCap(t *testing.T) {
	host, _ := newHost(t, map[string]any{"max_order_total": 1000})
	out := host.OrderPreConfirm(context.Background(), commerceext.OrderView{ID: "o1", TotalAmount: 1500})
	if out.Allow || out.Code != "LOYALTY_ORDER_CAP" {
		t.Fatalf("expected LOYALTY_ORDER_CAP, got %+v", out)
	}
}

func TestIntegrationPointsAreIdempotentUnderAtLeastOnce(t *testing.T) {
	host, p := newHost(t, nil)
	// O mesmo pedido chega em DOIS eventos com IDs distintos (dupla emissão
	// real). O dedupe por order_id credita uma vez só.
	errs := host.PublishAtLeastOnce(context.Background(), commerceext.Event{
		Type: commerceext.EventOrderConfirmed,
		Data: map[string]any{"order_id": "o1", "total_amount": float64(250)},
	})
	if len(errs) != 0 {
		t.Fatalf("unexpected handler errors: %v", errs)
	}
	if got := p.Points(); got != 250 {
		t.Fatalf("expected 250 points (uma vez), got %d", got)
	}
}

func TestIntegrationHandlerNeverExceedsHookTimeout(t *testing.T) {
	// Guarda de performance: com o teto do host (300ms) apertado para 50ms,
	// os handlers deste plugin continuam respondendo — se alguém introduzir
	// I/O bloqueante num hook, este teste degrada para PLUGIN_FAILURE.
	host, _ := newHost(t, nil)
	host.SetTimeout(50 * time.Millisecond)
	out := host.QuoteAdjust(context.Background(), commerceext.QuoteAdjustInput{Subtotal: 500})
	if !out.Allow {
		t.Fatalf("handler estourou o timeout do hook: %+v", out)
	}
}
