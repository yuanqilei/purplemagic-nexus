package logincodes

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"
)

func TestGiftRejectsInvalidConfigurationBeforeWriting(t *testing.T) {
	s := &Store{}
	valid := Gift{Name: "礼包", Campaign: "launch", MaxUses: 1, StartsAt: time.Now(), EndsAt: time.Now().Add(time.Hour), Reward: []byte(`{}`)}
	for _, v := range []struct {
		count  int
		phrase string
		change func(*Gift)
	}{
		{0, "", nil}, {1001, "", nil}, {2, "共享口令", nil}, {1, "a", nil},
		{1, "", func(g *Gift) { g.MaxUses = 0 }}, {1, "", func(g *Gift) { g.EndsAt = g.StartsAt }},
		{1, "", func(g *Gift) { g.Campaign = "" }},
	} {
		g := valid
		if v.change != nil {
			v.change(&g)
		}
		if _, err := s.CreateGifts(context.Background(), g, v.count, v.phrase); err == nil {
			t.Fatal("invalid gift accepted")
		}
	}
}

func TestGiftMySQLConcurrentAllowanceAndReplay(t *testing.T) {
	dsn := os.Getenv("DNFBRIDGE_PLAYER_TEST_DSN")
	if dsn == "" {
		t.Skip("disposable MySQL DSN not configured")
	}
	admin, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	name := fmt.Sprintf("gift_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4"); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec("DROP DATABASE `" + name + "`")
	cfg, err := driver.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DBName = name
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(20)
	s, err := NewStore(db, "accounts")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err = s.EnsureGiftSchema(ctx); err != nil {
		t.Fatal(err)
	}
	codes, err := s.CreateGifts(ctx, Gift{Name: "限量口令", Campaign: "launch", MaxUses: 3, StartsAt: time.Now().Add(-time.Minute), EndsAt: time.Now().Add(time.Hour), Reward: []byte(`{}`)}, 1, "公益开服口令")
	if err != nil {
		t.Fatal(err)
	}
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, _, e := s.ReserveGift(ctx, codes[0], fmt.Sprint(i), "1"); e == nil {
				accepted.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if accepted.Load() != 3 {
		t.Fatalf("accepted %d, want 3", accepted.Load())
	}
	claims, err := s.GiftClaims(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(claims) != 3 {
		t.Fatal(claims)
	}
	c := claims[0]
	if err = s.DisableGift(ctx, c.CodeID); err != nil {
		t.Fatal(err)
	}
	_, again, err := s.ReserveGift(ctx, codes[0], c.Account, c.Character)
	if err != nil || again.ID != c.ID {
		t.Fatalf("reserved retry lost: %v %v", again, err)
	}
	if _, _, err = s.ReserveGift(ctx, codes[0], c.Account, "other"); err == nil {
		t.Fatal("recipient switch accepted")
	}
	if err = s.FinishGift(ctx, c.ID, "mail-1"); err != nil {
		t.Fatal(err)
	}
	_, again, err = s.ReserveGift(ctx, codes[0], c.Account, c.Character)
	if err != nil || again.Status != "delivered" || again.MailID != "mail-1" {
		t.Fatalf("delivered replay lost: %v %v", again, err)
	}
}
