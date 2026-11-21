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
		{0, "", nil}, {1001, "", nil}, {2, "共享口令", nil}, {1, "   ", nil},
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
 if err=s.EnsureSchema(ctx); err!=nil { t.Fatal(err) }
 registration,err:=s.Generate(ctx,GenerateInput{Count:2,Batch:"persistence-test"},"fixture")
 if err!=nil { t.Fatal(err) }
 another,err:=NewStore(db,"accounts")
 if err!=nil { t.Fatal(err) }
 listed,err:=another.List(ctx,"","persistence-test",1,100)
 if err!=nil || len(listed.Items)!=2 || listed.Items[0].Code!=registration[1] { t.Fatal("registration plaintext not persisted",err) }
	codes, err := s.CreateGifts(ctx, Gift{Name: "限量口令", Campaign: "launch", MaxUses: 3, StartsAt: time.Now().Add(-time.Minute), EndsAt: time.Now().Add(time.Hour), Reward: []byte(`{}`)}, 1, "公益开服口令")
	if err != nil {
		t.Fatal(err)
	}
	// A fresh store instance must return the exact original plaintext after reload.
	reloaded, e := NewStore(db, "accounts")
	if e != nil {
		t.Fatal(e)
	}
	saved, e := reloaded.Gifts(ctx)
	if e != nil || len(saved) != 1 || saved[0].Code != codes[0] || saved[0].Kind != "phrase" {
		t.Fatal("plaintext phrase not persisted", e)
	}
	if e = s.EnsureGiftSchema(ctx); e != nil {
		t.Fatal("schema migration is not repeatable", e)
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
	batch, err := s.CreateGifts(ctx, Gift{Name: "随机礼包", Campaign: "random", MaxUses: 1, StartsAt: time.Now().Add(-time.Minute), EndsAt: time.Now().Add(time.Hour), Reward: []byte(`{}`)}, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range batch {
		for _, r := range code {
			if r < 'A' || r > 'Z' {
				t.Fatal("CDK contains a non-letter")
			}
		}
	}
	saved, e = reloaded.Gifts(ctx)
	if e != nil || len(saved) != 3 || saved[0].Code != batch[1] || saved[0].Kind != "cdk" {
		t.Fatal("plaintext CDK not persisted", e)
	}
	if _, _, err = s.ReserveGift(ctx, batch[0], "same-account", "1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.ReserveGift(ctx, batch[1], "same-account", "1"); err == nil {
		t.Fatal("second code bypassed campaign account limit")
	}
	if _, _, err = s.ReserveGift(ctx, batch[0], "other-account", "2"); err == nil {
		t.Fatal("single-use CDK reused by another account")
	}
	if _, _, err = s.ReserveGift(ctx, batch[1], "other-account", "2"); err != nil {
		t.Fatal("rejected claim consumed another code", err)
	}
	if _, err = db.Exec("CREATE TABLE accounts_tokens(token VARCHAR(64) PRIMARY KEY,account_id VARCHAR(64),expires_at DATETIME)"); err != nil {
		t.Fatal(err)
	}
	token := "de509f65000000000000000000000000"
	if _, err = db.Exec("INSERT INTO accounts_tokens VALUES (?,?,UTC_TIMESTAMP()+INTERVAL 1 HOUR)", token, "authenticated-account"); err != nil {
		t.Fatal(err)
	}
	if id, e := s.GiftAccount(ctx, token); e != nil || id != "authenticated-account" {
		t.Fatalf("token identity: %q %v", id, e)
	}
	if _, err = db.Exec("UPDATE accounts_tokens SET expires_at=UTC_TIMESTAMP()-INTERVAL 1 SECOND"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.GiftAccount(ctx, token); err == nil {
		t.Fatal("expired token accepted")
	}
}
