package logincodes

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const alphabet = "2346789ABCDEFGHJKMNPQRTUVWXY"
var prefixPattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,40}$`)

type Store struct { db *sql.DB; prefix string }

func NewStore(db *sql.DB, tablePrefix string) (*Store, error) {
	if db == nil || !prefixPattern.MatchString(tablePrefix) { return nil, errors.New("invalid registration-code store") }
	return &Store{db:db, prefix:tablePrefix+"_login_codes"}, nil
}

func (s *Store) EnsureSchema(ctx context.Context) error {
	for _, ddl := range []string{
		"CREATE TABLE IF NOT EXISTS "+s.prefix+" (id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, code_hash CHAR(64) NOT NULL, code_prefix CHAR(8) NOT NULL, batch_id VARCHAR(64) NOT NULL, status ENUM('issued','reserved','active','revoked') NOT NULL DEFAULT 'issued', bound_account VARCHAR(128) NULL, reserved_at DATETIME(6) NULL, max_uses INT UNSIGNED NOT NULL DEFAULT 1, used_count INT UNSIGNED NOT NULL DEFAULT 0, expires_at DATETIME(6) NULL, activated_at DATETIME(6) NULL, created_by VARCHAR(64) NOT NULL, note VARCHAR(200) NOT NULL DEFAULT '', created_at DATETIME(6) NOT NULL, UNIQUE KEY uk_code_hash(code_hash), KEY idx_batch(batch_id), KEY idx_status(status), KEY idx_bound(bound_account)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4",
		"CREATE TABLE IF NOT EXISTS "+s.prefix+"_audit (id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, code_id BIGINT UNSIGNED NOT NULL, action VARCHAR(32) NOT NULL, result VARCHAR(32) NOT NULL, actor VARCHAR(64) NOT NULL, ip VARCHAR(64) NOT NULL DEFAULT '', detail VARCHAR(200) NOT NULL DEFAULT '', at DATETIME(6) NOT NULL, KEY idx_code(code_id,at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4",
	} { if _, err := s.db.ExecContext(ctx, ddl); err != nil { return err } }
	return nil
}
func (s *Store) CheckSchema(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, "SELECT id FROM "+s.prefix+" LIMIT 0")
	if err != nil { return err }
	return rows.Close()
}

func Normalize(code string) (string, error) {
	v := strings.ToUpper(strings.TrimSpace(code)); v = strings.ReplaceAll(v," ",""); v = strings.ReplaceAll(v,"-","")
	if strings.HasPrefix(v,"DNF") { v = strings.TrimPrefix(v,"DNF") }
	if len(v)!=16 { return "", errors.New("注册码格式无效") }
	for _, c := range v { if !strings.ContainsRune(alphabet,c) { return "", errors.New("注册码格式无效") } }
	return v,nil
}
func hashCode(normal string) string { sum:=sha256.Sum256([]byte(normal)); return hex.EncodeToString(sum[:]) }
func formatCode(normal string) string { return "DNF-"+normal[:4]+"-"+normal[4:8]+"-"+normal[8:12]+"-"+normal[12:] }
func newCode() (string,error) { b:=make([]byte,16); out:=make([]byte,16); for i:=range out { for { if _,err:=rand.Read(b[:1]); err!=nil{return "",err}; if b[0]<252 { out[i]=alphabet[int(b[0])%len(alphabet)]; break } } }; return string(out),nil }

type GenerateInput struct { Count int `json:"count"`; Batch string `json:"batch"`; ExpiresAt *time.Time `json:"expiresAt"`; MaxUses int `json:"maxUses"`; Note string `json:"note"` }
type Code struct { ID uint64 `json:"id"`; Prefix string `json:"prefix"`; Batch string `json:"batch"`; Status string `json:"status"`; Account string `json:"account,omitempty"`; Used uint `json:"used"`; Max uint `json:"max"`; ExpiresAt *time.Time `json:"expiresAt,omitempty"`; Note string `json:"note"`; CreatedAt time.Time `json:"createdAt"` }
type Page struct { Total int `json:"total"`; Items []Code `json:"items"` }
func (s *Store) Generate(ctx context.Context, in GenerateInput, actor string) ([]string,error) {
	if in.MaxUses==0 { in.MaxUses=1 }
	if in.Count<1 || in.Count>1000 || in.MaxUses!=1 || len(in.Batch)>64 || len(in.Note)>200 || strings.TrimSpace(actor)=="" { return nil,errors.New("注册码生成参数无效") }
	if in.ExpiresAt!=nil && !in.ExpiresAt.After(time.Now()) { return nil,errors.New("注册码生成参数无效") }
	batch:=strings.TrimSpace(in.Batch); if batch=="" { batch=time.Now().UTC().Format("20060102T150405Z") }
	tx,err:=s.db.BeginTx(ctx,nil); if err!=nil{return nil,err}; defer tx.Rollback()
	codes:=make([]string,0,in.Count)
	for i:=0;i<in.Count;i++ { normal,e:=newCode(); if e!=nil{return nil,e}; shown:=formatCode(normal); result,e:=tx.ExecContext(ctx,"INSERT INTO "+s.prefix+" (code_hash,code_prefix,batch_id,max_uses,expires_at,created_by,note,created_at) VALUES (?,?,?,?,?,?,?,UTC_TIMESTAMP(6))",hashCode(normal),shown[:8],batch,in.MaxUses,in.ExpiresAt,actor,in.Note); if e!=nil{return nil,e}; id,_:=result.LastInsertId(); if _,e=tx.ExecContext(ctx,"INSERT INTO "+s.prefix+"_audit(code_id,action,result,actor,at) VALUES (?,'generate','ok',?,UTC_TIMESTAMP(6))",id,actor); e!=nil{return nil,e}; codes=append(codes,shown) }
	if err=tx.Commit(); err!=nil{return nil,err}; return codes,nil
}
func (s *Store) List(ctx context.Context, status,batch string,page,size int) (Page,error) {
	if page<1 {page=1}; if size<1 {size=50}; if size>100 {size=100}
	where:=" WHERE (?='' OR status=?) AND (?='' OR batch_id=?)"; var out Page
	if err:=s.db.QueryRowContext(ctx,"SELECT COUNT(*) FROM "+s.prefix+where,status,status,batch,batch).Scan(&out.Total); err!=nil{return out,err}
	rows,err:=s.db.QueryContext(ctx,"SELECT id,code_prefix,batch_id,status,COALESCE(bound_account,''),used_count,max_uses,expires_at,note,created_at FROM "+s.prefix+where+" ORDER BY id DESC LIMIT ? OFFSET ?",status,status,batch,batch,size,(page-1)*size); if err!=nil{return out,err}; defer rows.Close()
	out.Items=[]Code{}; for rows.Next(){ var v Code; if err=rows.Scan(&v.ID,&v.Prefix,&v.Batch,&v.Status,&v.Account,&v.Used,&v.Max,&v.ExpiresAt,&v.Note,&v.CreatedAt);err!=nil{return out,err}; out.Items=append(out.Items,v) }; return out,rows.Err()
}
func (s *Store) Action(ctx context.Context,id uint64, action,actor,ip string) error {
	if id==0 || (action!="revoke"&&action!="unbind"&&action!="resolve") {return errors.New("注册码操作无效")}
	tx,err:=s.db.BeginTx(ctx,nil); if err!=nil{return err}; defer tx.Rollback()
	var status string; if err=tx.QueryRowContext(ctx,"SELECT status FROM "+s.prefix+" WHERE id=? FOR UPDATE",id).Scan(&status);err!=nil{return err}
	var result sql.Result
	if action=="revoke" { result,err=tx.ExecContext(ctx,"UPDATE "+s.prefix+" SET status='revoked' WHERE id=?",id) } else if action=="resolve" { result,err=tx.ExecContext(ctx,"UPDATE "+s.prefix+" SET status='active',reserved_at=NULL,used_count=1,activated_at=UTC_TIMESTAMP(6) WHERE id=? AND status='reserved' AND reserved_at < UTC_TIMESTAMP(6)-INTERVAL 5 MINUTE",id) } else { result,err=tx.ExecContext(ctx,"UPDATE "+s.prefix+" SET status='issued',bound_account=NULL,reserved_at=NULL,used_count=0,activated_at=NULL WHERE id=? AND status IN ('active','revoked')",id) }
	if err!=nil{return err}; n,_:=result.RowsAffected(); if n!=1{return errors.New("注册码状态不允许此操作")}
	if _,err=tx.ExecContext(ctx,"INSERT INTO "+s.prefix+"_audit(code_id,action,result,actor,ip,at) VALUES (?,?, 'ok',?,?,UTC_TIMESTAMP(6))",id,action,actor,ip);err!=nil{return err}; return tx.Commit()
}
func (s *Store) Audit(ctx context.Context,id uint64,size int) ([]map[string]any,error) {
	if size<1||size>200 {size=100}; rows,err:=s.db.QueryContext(ctx,"SELECT action,result,actor,ip,detail,at FROM "+s.prefix+"_audit WHERE code_id=? ORDER BY id DESC LIMIT ?",id,size); if err!=nil{return nil,err}; defer rows.Close()
	out:=[]map[string]any{}; for rows.Next(){ var action,result,actor,ip,detail string; var at time.Time; if err=rows.Scan(&action,&result,&actor,&ip,&detail,&at);err!=nil{return nil,err}; out=append(out,map[string]any{"action":action,"result":result,"actor":actor,"ip":ip,"detail":detail,"at":at}) }; return out,rows.Err()
}

// Reserve locks one issued code to the requested account before forwarding a registration.
func (s *Store) Reserve(ctx context.Context, code, account, actor, ip string) (uint64,error) {
	normal,err:=Normalize(code); if err!=nil{return 0,err}; account=strings.TrimSpace(account); if account==""||len(account)>128{return 0,errors.New("账号无效")}
	tx,err:=s.db.BeginTx(ctx,nil); if err!=nil{return 0,err}; defer tx.Rollback()
	var id uint64; var status string; var bound sql.NullString; var expiry sql.NullTime; var max,used uint
	err=tx.QueryRowContext(ctx,"SELECT id,status,bound_account,expires_at,max_uses,used_count FROM "+s.prefix+" WHERE code_hash=? FOR UPDATE",hashCode(normal)).Scan(&id,&status,&bound,&expiry,&max,&used)
	if errors.Is(err,sql.ErrNoRows){return 0,errors.New("注册码不存在")}; if err!=nil{return 0,err}; if expiry.Valid&&time.Now().After(expiry.Time){return 0,errors.New("注册码已过期")}; if status=="revoked"{return 0,errors.New("注册码已停用")}; if status=="active"&&(!bound.Valid||!strings.EqualFold(bound.String,account)){return 0,errors.New("注册码已使用")}; if status=="reserved"&&(!bound.Valid||!strings.EqualFold(bound.String,account)){return 0,errors.New("注册码正在用于其他账号注册")}; if used>=max&&status!="reserved"{return 0,errors.New("注册码使用次数已用完")}
	if status=="issued" { if _,err=tx.ExecContext(ctx,"UPDATE "+s.prefix+" SET status='reserved',bound_account=?,reserved_at=UTC_TIMESTAMP(6) WHERE id=?",account,id);err!=nil{return 0,err} }
	if _,err=tx.ExecContext(ctx,"INSERT INTO "+s.prefix+"_audit(code_id,action,result,actor,ip,at) VALUES (?,'activate','reserved',?,?,UTC_TIMESTAMP(6))",id,actor,ip);err!=nil{return 0,err}; return id,tx.Commit()
}
func (s *Store) Finish(ctx context.Context,id uint64,account,actor,ip string,success bool) error {
	tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
	result:="invalid"; var updated sql.Result
	if success { updated,err=tx.ExecContext(ctx,"UPDATE "+s.prefix+" SET status='active',reserved_at=NULL,used_count=1,activated_at=UTC_TIMESTAMP(6) WHERE id=? AND status='reserved' AND bound_account=?",id,account); result="ok" } else { updated,err=tx.ExecContext(ctx,"UPDATE "+s.prefix+" SET status='issued',bound_account=NULL,reserved_at=NULL WHERE id=? AND status='reserved' AND bound_account=?",id,account) }
	if err!=nil{return err}; n,err:=updated.RowsAffected(); if err!=nil{return err}; if n!=1{return errors.New("注册码状态已变化")}
	if _,err=tx.ExecContext(ctx,"INSERT INTO "+s.prefix+"_audit(code_id,action,result,actor,ip,at) VALUES (?,'activate',?,?,?,UTC_TIMESTAMP(6))",id,result,actor,ip);err!=nil{return err}; return tx.Commit()
}
func (s *Store) String() string { return fmt.Sprintf("login-code-store(%s)",s.prefix) }
