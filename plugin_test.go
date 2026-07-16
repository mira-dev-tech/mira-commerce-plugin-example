package exampleloyalty

// Testes UNITÁRIOS do plugin: exercitam os handlers direto, sem subir o host.
// Note que só importamos commerce-ext — o teste de integração pela cadeia real
// do host vive em internal/ext/example_loyalty_test.go.

import (
	"context"
	"testing"

	commerceext "github.com/mira-dev-tech/commerce-ext"
)

func newInited(t *testing.T, cfg map[string]any) *Plugin {
	t.Helper()
	p := New()
	if err := p.Init(context.Background(), &commerceext.Runtime{Config: cfg}); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestQuoteAdjustBelowThresholdKeepsTotal(t *testing.T) {
	p := newInited(t, nil)
	out := p.onQuoteAdjust(context.Background(), commerceext.QuoteAdjustInput{Subtotal: 100})
	if !out.Allow || out.Discount != 0 || out.TotalAmount != 0 {
		t.Fatalf("expected no-op allow, got %+v", out)
	}
}

func TestQuoteAdjustAppliesDiscount(t *testing.T) {
	p := newInited(t, map[string]any{"min_subtotal": 300, "discount_pct": 5})
	out := p.onQuoteAdjust(context.Background(), commerceext.QuoteAdjustInput{Subtotal: 500})
	if !out.Allow {
		t.Fatalf("expected allow, got %+v", out)
	}
	if out.Discount != 25 || out.TotalAmount != 475 {
		t.Fatalf("expected 5%% de 500 (discount=25 total=475), got %+v", out)
	}
}

func TestPreConfirmBlocksAboveCap(t *testing.T) {
	p := newInited(t, map[string]any{"max_order_total": 1000})
	out := p.onPreConfirm(context.Background(), commerceext.OrderView{ID: "o1", TotalAmount: 1500})
	if out.Allow || out.Code != "LOYALTY_ORDER_CAP" {
		t.Fatalf("expected LOYALTY_ORDER_CAP deny, got %+v", out)
	}
	ok := p.onPreConfirm(context.Background(), commerceext.OrderView{ID: "o2", TotalAmount: 900})
	if !ok.Allow {
		t.Fatalf("expected allow below cap, got %+v", ok)
	}
}

func TestPreConfirmCapDisabledByDefault(t *testing.T) {
	p := newInited(t, nil)
	out := p.onPreConfirm(context.Background(), commerceext.OrderView{TotalAmount: 1e9})
	if !out.Allow {
		t.Fatalf("cap default é desligado (0), got %+v", out)
	}
}

func TestOrderConfirmedIsIdempotentByOrderID(t *testing.T) {
	p := newInited(t, nil)
	data := map[string]any{"order_id": "o1", "total_amount": float64(250)}
	// At-least-once na prática: o MESMO pedido chega em dois eventos com IDs
	// distintos (workflow engine + caminho direto do submit). O dedupe tem de
	// ser pela chave de negócio (order_id), não pelo Event.ID.
	for _, id := range []string{"evt-1", "evt-2"} {
		e := commerceext.Event{ID: id, Type: commerceext.EventOrderConfirmed, Data: data}
		if err := p.onOrderConfirmed(context.Background(), e); err != nil {
			t.Fatal(err)
		}
	}
	if got := p.Points(); got != 250 {
		t.Fatalf("expected 250 points (creditado uma vez), got %d", got)
	}
	// Pedido diferente credita normalmente.
	e := commerceext.Event{ID: "evt-3", Type: commerceext.EventOrderConfirmed,
		Data: map[string]any{"order_id": "o2", "total_amount": float64(100)}}
	if err := p.onOrderConfirmed(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	if got := p.Points(); got != 350 {
		t.Fatalf("expected 350 points, got %d", got)
	}
}

func TestRegisterDeclaresManifestCapabilities(t *testing.T) {
	// O que Register declara tem de bater com capabilities do manifest.yaml.
	p := newInited(t, nil)
	reg := commerceext.NewRegistry(p.Meta().ID)
	if err := p.Register(reg); err != nil {
		t.Fatal(err)
	}
	hooks := map[string]bool{}
	for _, h := range reg.Hooks() {
		hooks[h.HookID] = true
	}
	if !hooks[commerceext.HookCheckoutQuoteAdjust] || !hooks[commerceext.HookOrderPreConfirm] {
		t.Fatalf("hooks registrados divergem do manifest: %v", hooks)
	}
	if len(reg.Events()) != 1 || reg.Events()[0].EventType != commerceext.EventOrderConfirmed {
		t.Fatalf("eventos registrados divergem do manifest: %+v", reg.Events())
	}
}
