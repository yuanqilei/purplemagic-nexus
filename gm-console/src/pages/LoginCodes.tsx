import { useEffect, useState } from "react";
import { KeyRound, RefreshCw, Ban, Unlink, Check } from "lucide-react";
import { useLive } from "../api/context";
import type { PageProps } from "../ui";
import "../login-codes.css";

type Code = {id:number;prefix:string;batch:string;status:string;account?:string;used:number;max:number;expiresAt?:string;note:string;createdAt:string};
export function LoginCodes({notify}:PageProps) {
  const api=useLive()!, [items,setItems]=useState<Code[]>([]), [codes,setCodes]=useState<string[]>([]), [count,setCount]=useState(20), [batch,setBatch]=useState(""), [note,setNote]=useState(""), [busy,setBusy]=useState(false);
  const refresh=async()=>{setBusy(true);try{const r=await api.http.fresh<{total:number;items:Code[]}>("/api/v1/login-codes");setItems(r.data.items)}catch(e){notify(String(e))}finally{setBusy(false)}};
  useEffect(()=>{void refresh()},[]);
  const generate=async()=>{setBusy(true);try{const r=await api.http.post<{batch:string;codes:string[]}>("/api/v1/login-codes",{count,batch,note,maxUses:1});setCodes(r.codes);await refresh()}catch(e){notify(String(e))}finally{setBusy(false)}};
  const action=async(id:number,verb:"revoke"|"unbind"|"resolve")=>{const verbLabel=verb==="revoke"?"停用":verb==="unbind"?"解绑并重新发放":"等待五分钟并确认账号已创建后核销此码";if(!window.confirm(`确认${verbLabel} #${id}？`))return;try{await api.http.post(`/api/v1/login-codes/${id}/${verb}`,{});await refresh()}catch(e){notify(String(e))}};
  return <section className="page-stack login-codes">
    <div className="glass content-panel"><div className="panel-heading"><div><p className="eyebrow">ACCOUNT ACCESS</p><h2>注册码</h2><p>生成一次性注册码，玩家注册成功后自动绑定账号。</p></div><KeyRound/></div>
      <div className="code-create"><label>生成数量<input type="number" min={1} max={1000} value={count} onChange={e=>setCount(Math.max(1,Math.min(1000,Number(e.target.value))))}/></label><label>批次<input value={batch} onChange={e=>setBatch(e.target.value)} placeholder="留空自动命名"/></label><label>备注<input value={note} onChange={e=>setNote(e.target.value)} maxLength={200}/></label><button className="primary" disabled={busy} onClick={()=>void generate()}>{busy?"正在生成…":"生成注册码"}</button></div>
      {codes.length>0&&<div className="code-result"><strong>注册码只显示这一次，请立即保存</strong><textarea readOnly value={codes.join("\n")} onFocus={e=>e.currentTarget.select()}/><button className="secondary" onClick={()=>{void navigator.clipboard.writeText(codes.join("\n"));notify("注册码已复制")}}>复制全部</button></div>}
    </div>
    <div className="table-panel glass"><div className="panel-heading"><div><h2>已生成注册码</h2><p>列表仅显示前缀；完整注册码不会再次返回。</p></div><button className="secondary" disabled={busy} onClick={()=>void refresh()}><RefreshCw size={15}/>刷新</button></div>
      <div className="table-scroll"><table><thead><tr><th>前缀</th><th>批次</th><th>状态</th><th>绑定账号</th><th>次数</th><th>备注</th><th>操作</th></tr></thead><tbody>{items.map(c=><tr key={c.id}><td>{c.prefix}…</td><td>{c.batch}</td><td>{c.status}</td><td>{c.account||"—"}</td><td>{c.used}/{c.max}</td><td>{c.note}</td><td>{c.status==="reserved"&&<button className="icon-button" title="核实账号已创建后核销" onClick={()=>void action(c.id,"resolve")}><Check size={15}/></button>}<button className="icon-button" title="停用" onClick={()=>void action(c.id,"revoke")}><Ban size={15}/></button>{(c.status==="active"||c.status==="revoked")&&<button className="icon-button" title="解绑并重新发放" onClick={()=>void action(c.id,"unbind")}><Unlink size={15}/></button>}</td></tr>)}{items.length===0&&<tr><td colSpan={7}>暂无注册码</td></tr>}</tbody></table></div>
    </div>
  </section>
}
