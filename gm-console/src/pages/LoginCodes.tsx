import { useEffect, useState } from "react";
import { Ban, Check, RefreshCw, Unlink } from "lucide-react";
import { useLive } from "../api/context";
import type { PageProps } from "../ui";
import "../login-codes.css";

type Code = {id:number;code:string;prefix:string;batch:string;status:string;account?:string;used:number;max:number;expiresAt?:string;note:string;activatedAt?:string;createdAt:string};
type Audit = {action:string;result:string;actor:string;detail:string;at:string};

export function LoginCodes({notify}:PageProps) {
  const api=useLive()!;
  const [view,setView]=useState<"policy"|"manage">("manage");
  const [items,setItems]=useState<Code[]>([]),[codes,setCodes]=useState<string[]>([]),[count,setCount]=useState(20),[batch,setBatch]=useState(""),[note,setNote]=useState(""),[busy,setBusy]=useState(false);
  const [required,setRequired]=useState<boolean|null>(null),[search,setSearch]=useState(""),[status,setStatus]=useState(""),[page,setPage]=useState(1),[total,setTotal]=useState(0),[audit,setAudit]=useState<Audit[]>([]);

  const openAccount=async(name:string)=>{try{const r=await api.quickSearch(name);const account=r.accounts.find(a=>a.name===name);if(!account){notify("未找到该账号");return}location.hash="accounts";await api.openAccountPanel(account.id)}catch(e){notify(String(e))}};
  const showAudit=async(id:number)=>{try{const r=await api.http.fresh<Audit[]>(`/api/v1/login-codes/${id}/audit`);setAudit(r.data)}catch(e){notify(String(e))}};
  const toggle=async()=>{if(required===null||!window.confirm(required?"关闭后玩家无需注册码即可注册，确认关闭？":"确认开启注册码校验？"))return;setBusy(true);try{const r=await api.http.post<{required:boolean}>("/api/v1/login-code-policy",{required:!required});setRequired(r.required)}catch(e){notify(String(e))}finally{setBusy(false)}};
  const refresh=async()=>{setBusy(true);try{const query=new URLSearchParams({page:String(page),size:"100",q:search,status});const r=await api.http.fresh<{total:number;items:Code[]}>(`/api/v1/login-codes?${query}`);setItems(r.data.items);setTotal(r.data.total);const policy=await api.http.fresh<{required:boolean}>("/api/v1/login-code-policy");setRequired(policy.data.required)}catch(e){notify(String(e))}finally{setBusy(false)}};
  useEffect(()=>{void refresh()},[page,search,status]);
  const generate=async()=>{setBusy(true);try{const r=await api.http.post<{batch:string;codes:string[]}>("/api/v1/login-codes",{count,batch,note,maxUses:1});setCodes(r.codes);await refresh()}catch(e){notify(String(e))}finally{setBusy(false)}};
  const action=async(id:number,verb:"revoke"|"unbind"|"resolve")=>{const label=verb==="revoke"?"停用":verb==="unbind"?"解绑并重新发放":"等待五分钟并确认账号已创建后核销此码";if(!window.confirm(`确认${label} #${id}？`))return;try{await api.http.post(`/api/v1/login-codes/${id}/${verb}`,{});await refresh()}catch(e){notify(String(e))}};

  return <section className="page-stack login-codes redemption-content">
    <div className="table-panel">
      <div className="redemption-tabs redemption-inner-tabs" role="tablist" aria-label="注册码设置">
        <button role="tab" aria-selected={view==="policy"} className={view==="policy"?"active":""} onClick={()=>setView("policy")}>注册限制</button>
        <button role="tab" aria-selected={view==="manage"} className={view==="manage"?"active":""} onClick={()=>setView("manage")}>注册码管理</button>
      </div>
      {view==="policy"? <div className="redemption-policy-panel">
        <h3>注册限制</h3><p>控制新账号注册时是否必须提交注册码。</p>
        <div className="redemption-toolbar"><span>当前策略</span><strong className={required?"policy-enabled":"policy-disabled"}>{required===null?"正在读取…":required?"注册码校验已开启":"无需注册码即可注册"}</strong><span className="redemption-spacer"/><button className={required?"secondary":"primary"} disabled={busy||required===null} onClick={()=>void toggle()}>{required?"关闭注册码校验":"开启注册码校验"}</button></div>
      </div> : <>
        <div className="redemption-generator">
          <h3>生成注册码</h3>
          <div className="redemption-generator-fields">
            <label>生成数量<input type="number" min={1} max={1000} value={count} onChange={e=>setCount(Math.max(1,Math.min(1000,Number(e.target.value))))}/></label>
            <label>注册码长度<input value="16 位字符（固定）" readOnly/></label>
            <label>批次<input value={batch} onChange={e=>setBatch(e.target.value)} placeholder="留空自动命名"/></label>
            <label>备注<input value={note} onChange={e=>setNote(e.target.value)} maxLength={200}/></label>
          </div>
          <div className="redemption-generator-actions"><button className="primary" disabled={busy} onClick={()=>void generate()}>{busy?"正在生成…":"生成注册码"}</button></div>
          {codes.length>0&&<div className="code-result"><strong>注册码已保存，可在列表长期查看</strong><textarea readOnly value={codes.join("\n")} onFocus={e=>e.currentTarget.select()}/><button className="secondary" onClick={()=>{void navigator.clipboard.writeText(codes.join("\n"));notify("注册码已复制")}}>复制全部</button></div>}
        </div>
        <div className="redemption-toolbar">
          <input aria-label="搜索账号、注册码或批次" placeholder="搜索账号、注册码或批次" value={search} onChange={e=>{setSearch(e.target.value);setPage(1)}}/>
          <select aria-label="注册码状态" value={status} onChange={e=>{setStatus(e.target.value);setPage(1)}}><option value="">全部状态</option><option value="issued">未使用</option><option value="active">已使用</option><option value="reserved">待确认</option><option value="revoked">已停用</option></select>
          <button className="secondary" disabled={busy} onClick={()=>void refresh()}>搜索</button>
          <button className="secondary" disabled={busy} onClick={()=>void refresh()}><RefreshCw size={15}/>刷新</button>
          <span className="redemption-count">共 {total} 条</span>
        </div>
        <div className="table-scroll"><table><thead><tr><th>编号</th><th>注册码</th><th>状态</th><th>使用账号</th><th>使用时间</th><th>次数</th><th>批次 / 备注</th><th>操作</th></tr></thead><tbody>
          {items.map(c=><tr key={c.id}><td>{c.id}</td><td>{c.code||`${c.prefix}…（历史码无明文）`}</td><td>{c.status==="issued"?"未使用":c.status==="active"?"已使用":c.status==="reserved"?"待确认":"已停用"}</td><td>{c.account?<button className="secondary" onClick={()=>void openAccount(c.account!)}>{c.account}</button>:"—"}</td><td>{c.activatedAt?new Date(c.activatedAt).toLocaleString():"—"}</td><td>{c.used}/{c.max}</td><td>{c.batch}{c.note?` · ${c.note}`:""}</td><td><button className="secondary" onClick={()=>void showAudit(c.id)}>使用记录</button>{c.status==="reserved"&&<button className="icon-button" title="核实账号已创建后核销" onClick={()=>void action(c.id,"resolve")}><Check size={15}/></button>}<button className="icon-button" title="停用" onClick={()=>void action(c.id,"revoke")}><Ban size={15}/></button>{(c.status==="active"||c.status==="revoked")&&<button className="icon-button" title="解绑并重新发放" onClick={()=>void action(c.id,"unbind")}><Unlink size={15}/></button>}</td></tr>)}
          {items.length===0&&<tr><td colSpan={8}>暂无注册码</td></tr>}
        </tbody></table></div>
        <div className="redemption-toolbar redemption-pagination"><span>第 {page} 页</span><span className="redemption-spacer"/><button className="secondary" disabled={busy||page===1} onClick={()=>setPage(page-1)}>上一页</button><button className="secondary" disabled={busy||page*100>=total} onClick={()=>setPage(page+1)}>下一页</button></div>
        <details className="redemption-audit"><summary>注册码操作记录{audit.length?` · ${audit.length} 条`:""}</summary>{audit.map((a,i)=><p key={i}>{new Date(a.at).toLocaleString()} · {a.action} · {a.result} · {a.actor} · {a.detail}</p>)}</details>
      </>}
    </div>
  </section>;
}
