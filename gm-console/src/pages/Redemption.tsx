import {useState} from "react";
import {Gifts} from "./Gifts";
import {LoginCodes} from "./LoginCodes";
import type {PageProps} from "../ui";
import "../redemption.css";
export function Redemption(props:PageProps){
 const [tab,setTab]=useState<"cdk"|"phrase"|"register">("cdk");
 return <section className="page-stack redemption-page">
  <header className="redemption-heading"><div><h2>激活码</h2><p>管理激活码、口令码与注册码</p></div></header>
  <div className="redemption-tabs-shell"><div className="redemption-tabs" role="tablist" aria-label="激活码分类">{([["cdk","激活码"],["phrase","口令码"],["register","注册码"]] as const).map(([id,label])=><button role="tab" aria-selected={tab===id} className={tab===id?"active":""} key={id} onClick={()=>setTab(id)}>{label}</button>)}</div></div>
  {tab==="register"?<LoginCodes {...props}/>:<Gifts key={tab} {...props} mode={tab}/>}
 </section>
}
