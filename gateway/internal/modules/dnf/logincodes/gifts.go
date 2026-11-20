package logincodes

import (
 "context"
 "crypto/sha256"
 "database/sql"
 "encoding/hex"
 "encoding/json"
 "errors"
 "strings"
 "time"
)

// Gift codes share the registration database, but never registration-code rows.
type Gift struct {
 ID uint64 `json:"id"`
 Name string `json:"name"`
 Campaign string `json:"campaign"`
 Prefix string `json:"prefix"`
 MaxUses int `json:"maxUses"`
 Used int `json:"used"`
 Disabled bool `json:"disabled"`
 StartsAt time.Time `json:"startsAt"`
 EndsAt time.Time `json:"endsAt"`
 Reward json.RawMessage `json:"reward"`
}
type GiftClaim struct {
 ID string `json:"id"`
 CodeID uint64 `json:"codeId"`
 Account string `json:"accountId"`
 Character string `json:"characterId"`
 Status string `json:"status"`
 MailID string `json:"mailId"`
 CreatedAt time.Time `json:"createdAt"`
}
func(s *Store) EnsureGiftSchema(ctx context.Context)error{
 for _,q:=range []string{
 "CREATE TABLE IF NOT EXISTS "+s.prefix+"_gifts(id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,code_hash CHAR(64) NOT NULL UNIQUE,prefix VARCHAR(32) NOT NULL,name VARCHAR(100) NOT NULL,campaign VARCHAR(64) NOT NULL,max_uses INT NOT NULL,used INT NOT NULL DEFAULT 0,disabled BOOLEAN NOT NULL DEFAULT FALSE,starts_at DATETIME(6) NOT NULL,ends_at DATETIME(6) NOT NULL,reward LONGTEXT NOT NULL) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4",
 "CREATE TABLE IF NOT EXISTS "+s.prefix+"_gift_claims(id CHAR(64) PRIMARY KEY,code_id BIGINT UNSIGNED NOT NULL,campaign VARCHAR(64) NOT NULL,account_id VARCHAR(128) NOT NULL,character_id VARCHAR(64) NOT NULL,status VARCHAR(16) NOT NULL,mail_id VARCHAR(128) NOT NULL DEFAULT '',created_at DATETIME(6) NOT NULL,UNIQUE KEY campaign_account(campaign,account_id),KEY code_claim(code_id,created_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4",
 } {if _,err:=s.db.ExecContext(ctx,q);err!=nil{return err}};return nil
}
func giftHash(code string)string{h:=sha256.Sum256([]byte(strings.ToUpper(strings.TrimSpace(code))));return hex.EncodeToString(h[:])}
func(s *Store) CreateGifts(ctx context.Context,g Gift,count int,phrase string)([]string,error){
 if count<1||count>1000||len(g.Name)==0||len(g.Name)>100||len(g.Campaign)==0||len(g.Campaign)>64||g.MaxUses<1||!g.EndsAt.After(g.StartsAt)||!g.EndsAt.After(time.Now())||len(g.Reward)==0{return nil,errors.New("礼包参数无效")}
 if phrase!=""&&(count!=1||len(phrase)>128||len(strings.TrimSpace(phrase))<4){return nil,errors.New("口令长度或数量无效")}
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return nil,err};defer tx.Rollback();out:=[]string{}
 for i:=0;i<count;i++{code:=strings.TrimSpace(phrase);if code==""{n,e:=newCode();if e!=nil{return nil,e};code="GIFT-"+n};prefix:=[]rune(code);if len(prefix)>8{prefix=prefix[:8]};_,err=tx.ExecContext(ctx,"INSERT INTO "+s.prefix+"_gifts(code_hash,prefix,name,campaign,max_uses,starts_at,ends_at,reward) VALUES (?,?,?,?,?,?,?,?)",giftHash(code),string(prefix),g.Name,g.Campaign,g.MaxUses,g.StartsAt.UTC(),g.EndsAt.UTC(),string(g.Reward));if err!=nil{return nil,err};out=append(out,code)};return out,tx.Commit()
}
func scanGift(row interface{Scan(...any)error})(Gift,error){var g Gift;var raw string;err:=row.Scan(&g.ID,&g.Name,&g.Campaign,&g.Prefix,&g.MaxUses,&g.Used,&g.Disabled,&g.StartsAt,&g.EndsAt,&raw);g.Reward=json.RawMessage(raw);return g,err}
const giftColumns="id,name,campaign,prefix,max_uses,used,disabled,starts_at,ends_at,reward"
func(s *Store) Gifts(ctx context.Context)([]Gift,error){rows,err:=s.db.QueryContext(ctx,"SELECT "+giftColumns+" FROM "+s.prefix+"_gifts ORDER BY id DESC LIMIT 1000");if err!=nil{return nil,err};defer rows.Close();out:=[]Gift{};for rows.Next(){g,e:=scanGift(rows);if e!=nil{return nil,e};out=append(out,g)};return out,rows.Err()}
func(s *Store) DisableGift(ctx context.Context,id uint64)error{_,err:=s.db.ExecContext(ctx,"UPDATE "+s.prefix+"_gifts SET disabled=TRUE WHERE id=?",id);return err}
func(s *Store) GiftAccount(ctx context.Context,token string)(string,error){if len(token)!=32{return "",errors.New("请重新登录")};var account string;table:=strings.TrimSuffix(s.prefix,"_login_codes")+"_tokens";err:=s.db.QueryRowContext(ctx,"SELECT account_id FROM "+table+" WHERE token=? AND expires_at>UTC_TIMESTAMP()",token).Scan(&account);return account,err}
// Reserve the entitlement before dispatch. A failed delivery retains its slot and
// immutable recipient; retry uses the same permanent mail receipt.
func(s *Store) ReserveGift(ctx context.Context,code,account,character string)(Gift,GiftClaim,error){
 var c GiftClaim;tx,err:=s.db.BeginTx(ctx,&sql.TxOptions{Isolation:sql.LevelReadCommitted});if err!=nil{return Gift{},c,err};defer tx.Rollback()
 g,err:=scanGift(tx.QueryRowContext(ctx,"SELECT "+giftColumns+" FROM "+s.prefix+"_gifts WHERE code_hash=? FOR UPDATE",giftHash(code)));if err!=nil{return g,c,errors.New("礼包码不存在")}
 err=tx.QueryRowContext(ctx,"SELECT id,code_id,account_id,character_id,status,mail_id,created_at FROM "+s.prefix+"_gift_claims WHERE campaign=? AND account_id=?",g.Campaign,account).Scan(&c.ID,&c.CodeID,&c.Account,&c.Character,&c.Status,&c.MailID,&c.CreatedAt)
 if err==nil{if c.CodeID!=g.ID||c.Character!=character{return g,c,errors.New("本账号已领取该活动，重试需使用原兑换码及角色")};return g,c,tx.Commit()};if !errors.Is(err,sql.ErrNoRows){return g,c,err}
 now:=time.Now().UTC();if g.Disabled||now.Before(g.StartsAt)||!now.Before(g.EndsAt)||g.Used>=g.MaxUses{return g,c,errors.New("礼包未开始、已结束、已停用或已领完")}
 c=GiftClaim{ID:giftHash(g.Campaign+"|"+account),CodeID:g.ID,Account:account,Character:character,Status:"pending",CreatedAt:now}
 _,err=tx.ExecContext(ctx,"INSERT INTO "+s.prefix+"_gift_claims(id,code_id,campaign,account_id,character_id,status,created_at) VALUES (?,?,?,?,?,'pending',?)",c.ID,c.CodeID,g.Campaign,account,character,now);if err!=nil{return g,c,err};_,err=tx.ExecContext(ctx,"UPDATE "+s.prefix+"_gifts SET used=used+1 WHERE id=?",g.ID);if err!=nil{return g,c,err};return g,c,tx.Commit()
}
func(s *Store) FinishGift(ctx context.Context,id,mail string)error{_,err:=s.db.ExecContext(ctx,"UPDATE "+s.prefix+"_gift_claims SET status='delivered',mail_id=? WHERE id=?",mail,id);return err}
func(s *Store) GiftClaims(ctx context.Context)([]GiftClaim,error){rows,err:=s.db.QueryContext(ctx,"SELECT id,code_id,account_id,character_id,status,mail_id,created_at FROM "+s.prefix+"_gift_claims ORDER BY created_at DESC LIMIT 1000");if err!=nil{return nil,err};defer rows.Close();out:=[]GiftClaim{};for rows.Next(){var c GiftClaim;if err=rows.Scan(&c.ID,&c.CodeID,&c.Account,&c.Character,&c.Status,&c.MailID,&c.CreatedAt);err!=nil{return nil,err};out=append(out,c)};return out,rows.Err()}
