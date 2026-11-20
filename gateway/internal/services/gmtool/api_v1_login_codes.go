package gmtool

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dnf90.local/gateway/internal/modules/dnf/logincodes"
)

func (s *Server) v1GenerateLoginCodes(w http.ResponseWriter, r *http.Request) {
	if s.deps.LoginCodes == nil { v1Fail(w,r,503,"unavailable","注册码服务暂不可用",true); return }
	in,ok:=decodeBody[logincodes.GenerateInput](w,r); if !ok { return }
	if strings.TrimSpace(in.Batch)=="" { in.Batch=time.Now().UTC().Format("20060102T150405Z") }
	codes,err:=s.deps.LoginCodes.Generate(r.Context(),in,"admin")
	if err!=nil { if err.Error()=="注册码生成参数无效" { v1Fail(w,r,400,"invalid_request",err.Error(),false) } else { s.v1Unavailable(w,r,err) }; return }
	v1Reply(w,r,200,v1Envelope{Data:map[string]any{"batch":in.Batch,"codes":codes}})
}
func (s *Server) v1LoginCodes(w http.ResponseWriter,r *http.Request) {
	if s.deps.LoginCodes==nil {v1Fail(w,r,503,"unavailable","注册码服务暂不可用",true);return}
	q:=r.URL.Query(); page,_:=strconv.Atoi(q.Get("page")); size,_:=strconv.Atoi(q.Get("size"))
	out,err:=s.deps.LoginCodes.List(r.Context(),q.Get("status"),q.Get("batch"),page,size,q.Get("q"))
	if err!=nil {s.v1Unavailable(w,r,err);return}; v1Reply(w,r,200,v1Envelope{Data:out})
}
func (s *Server) v1LoginCodeAction(w http.ResponseWriter,r *http.Request) {
	if s.deps.LoginCodes==nil {v1Fail(w,r,503,"unavailable","注册码服务暂不可用",true);return}
	id,err:=strconv.ParseUint(r.PathValue("codeId"),10,64); if err!=nil {v1Fail(w,r,400,"invalid_request","注册码编号无效",false);return}
	action:=r.PathValue("action"); if action!="revoke"&&action!="unbind"&&action!="resolve" {v1Fail(w,r,404,"not_found","接口不存在",false);return}
	if err=s.deps.LoginCodes.Action(r.Context(),id,action,"admin",clientIP(r));err!=nil {if strings.Contains(err.Error(),"注册码状态")||err.Error()=="注册码操作无效" {v1Fail(w,r,409,"conflict",err.Error(),false)}else{s.v1Unavailable(w,r,err)};return}
	v1Reply(w,r,200,v1Envelope{Data:map[string]bool{"ok":true}})
}
func (s *Server) v1LoginCodeAudit(w http.ResponseWriter,r *http.Request) {
	if s.deps.LoginCodes==nil {v1Fail(w,r,503,"unavailable","注册码服务暂不可用",true);return}
	id,err:=strconv.ParseUint(r.PathValue("codeId"),10,64); if err!=nil {v1Fail(w,r,400,"invalid_request","注册码编号无效",false);return}
	size,_:=strconv.Atoi(r.URL.Query().Get("size")); ctx,cancel:=context.WithTimeout(r.Context(),5*time.Second); defer cancel()
	rows,err:=s.deps.LoginCodes.Audit(ctx,id,size); if err!=nil {s.v1Unavailable(w,r,err);return}; v1Reply(w,r,200,v1Envelope{Data:rows})
}

func(s *Server) v1LoginCodePolicy(w http.ResponseWriter,r *http.Request){if s.deps.LoginCodes==nil{v1Fail(w,r,503,"unavailable","注册码服务暂不可用",true);return};if r.Method=="POST"{p,ok:=decodeBody[logincodes.Policy](w,r);if !ok{return};if err:=s.deps.LoginCodes.SetPolicy(r.Context(),p);err!=nil{s.v1Unavailable(w,r,err);return}};p,err:=s.deps.LoginCodes.Policy(r.Context());if err!=nil{s.v1Unavailable(w,r,err);return};v1Reply(w,r,200,v1Envelope{Data:p})}
