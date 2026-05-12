package engine

import (
	"reflect"
	"testing"
)

func place(id, side, price, qty string, ts int64) Event {
	return Event{Type: "place", OrderID: id, Side: side, Price: price, Quantity: qty, Timestamp: ts}
}

func cancel(id string, ts int64) Event {
	return Event{Type: "cancel", OrderID: id, Timestamp: ts}
}

func step(t *testing.T, e *Engine, ev Event) []Trade {
	t.Helper()
	tr, err := e.Step(ev)
	if err != nil {
		t.Fatalf("step %+v: %v", ev, err)
	}
	// Copy because the engine reuses its internal slice.
	out := make([]Trade, len(tr))
	copy(out, tr)
	return out
}

func TestPlaceRestsWhenNoCross(t *testing.T) {
	e := New()
	got := step(t, e, place("o1", "buy", "100", "1", 1))
	if len(got) != 0 {
		t.Fatalf("expected no trades, got %v", got)
	}
	if e.bids.best().Price != 100*Scale {
		t.Fatalf("best bid = %d, want %d", e.bids.best().Price, 100*Scale)
	}
}

func TestSimpleMatch(t *testing.T) {
	e := New()
	step(t, e, place("m1", "sell", "100", "1", 1))
	got := step(t, e, place("t1", "buy", "100", "1", 2))
	want := []Trade{{
		TakerOrderID: "t1", MakerOrderID: "m1",
		Price: "100", Quantity: "1", Timestamp: 2,
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if e.asks.best() != nil {
		t.Fatal("ask side should be empty after full fill")
	}
	if _, ok := e.orders["m1"]; ok {
		t.Fatal("filled maker should be removed from orders map")
	}
}

func TestPartialFillTakerRests(t *testing.T) {
	e := New()
	step(t, e, place("m1", "sell", "100", "1", 1))
	got := step(t, e, place("t1", "buy", "100", "3", 2))
	if len(got) != 1 || got[0].Quantity != "1" {
		t.Fatalf("expected one fill of 1, got %+v", got)
	}
	// Taker should rest with remaining 2 @ 100 on bid side.
	if e.bids.best().Price != 100*Scale || e.bids.best().Orders[0].Quantity != 2*Scale {
		t.Fatalf("expected resting taker with qty 2, got %+v", e.bids.best())
	}
}

func TestPartialFillMakerStays(t *testing.T) {
	e := New()
	step(t, e, place("m1", "sell", "100", "5", 1))
	got := step(t, e, place("t1", "buy", "100", "2", 2))
	if len(got) != 1 || got[0].Quantity != "2" {
		t.Fatalf("expected one fill of 2, got %+v", got)
	}
	// Maker should still be at the front of asks with qty 3.
	if e.asks.best().Orders[0].Quantity != 3*Scale {
		t.Fatalf("maker remaining = %d, want %d", e.asks.best().Orders[0].Quantity, 3*Scale)
	}
}

func TestPriceTimePriority(t *testing.T) {
	// Two makers at same price: earliest fills first.
	e := New()
	step(t, e, place("m1", "sell", "100", "1", 1))
	step(t, e, place("m2", "sell", "100", "1", 2))
	got := step(t, e, place("t1", "buy", "100", "1", 3))
	if len(got) != 1 || got[0].MakerOrderID != "m1" {
		t.Fatalf("expected m1 to fill first, got %+v", got)
	}
}

func TestPricePrioritySell(t *testing.T) {
	// Buyer crosses; should hit the best (lowest) ask first.
	e := New()
	step(t, e, place("m_high", "sell", "101", "1", 1))
	step(t, e, place("m_low", "sell", "100", "1", 2))
	got := step(t, e, place("t1", "buy", "101", "2", 3))
	if len(got) != 2 {
		t.Fatalf("expected 2 fills, got %+v", got)
	}
	if got[0].MakerOrderID != "m_low" || got[0].Price != "100" {
		t.Errorf("first fill should be m_low @ 100, got %+v", got[0])
	}
	if got[1].MakerOrderID != "m_high" || got[1].Price != "101" {
		t.Errorf("second fill should be m_high @ 101, got %+v", got[1])
	}
}

func TestNoCrossOnPriceMismatch(t *testing.T) {
	e := New()
	step(t, e, place("m1", "sell", "101", "1", 1))
	got := step(t, e, place("t1", "buy", "100", "1", 2))
	if len(got) != 0 {
		t.Fatalf("expected no fills (100 < 101), got %+v", got)
	}
}

func TestCancelRestingOrder(t *testing.T) {
	e := New()
	step(t, e, place("o1", "buy", "100", "1", 1))
	step(t, e, cancel("o1", 2))
	if e.bids.best() != nil {
		t.Fatal("book should be empty after cancel")
	}
	if _, ok := e.orders["o1"]; ok {
		t.Fatal("orders map should not contain canceled order")
	}
}

func TestCancelUnknownIsNoOp(t *testing.T) {
	e := New()
	if _, err := e.Step(cancel("ghost", 1)); err != nil {
		t.Fatalf("cancel of unknown id should be no-op, got %v", err)
	}
}

func TestDuplicatePlaceRejected(t *testing.T) {
	e := New()
	step(t, e, place("o1", "buy", "100", "1", 1))
	if _, err := e.Step(place("o1", "buy", "100", "1", 2)); err == nil {
		t.Fatal("expected error on duplicate order id")
	}
}

func TestFractionalQuantities(t *testing.T) {
	e := New()
	step(t, e, place("m1", "sell", "50000.50", "1.5", 1))
	got := step(t, e, place("t1", "buy", "50000.50", "0.5", 2))
	want := []Trade{{
		TakerOrderID: "t1", MakerOrderID: "m1",
		Price: "50000.5", Quantity: "0.5", Timestamp: 2,
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
