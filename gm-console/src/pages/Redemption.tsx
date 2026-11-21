import {useState} from "react";
import {Gifts} from "./Gifts";
import {LoginCodes} from "./LoginCodes";
import type {PageProps} from "../ui";
export function Redemption(props:PageProps){
 const [tab,setTab]=useState<"cdk"|"phrase"|"register">("cdk");
 return <section className="page-stack"><div className="glass content-panel"><h2>兑换与注册</h2><div className="code-create" role="tablist" aria-label="兑换与注册分类">{([["cdk","CDK"],["phrase","口令"],["register","注册码"]] as const).map(([id,label])=><button role="tab" aria-selected={tab===id} className={tab===id?"primary":"secondary"} key={id} onClick={()=>setTab(id)}>{label}</button>)}</div></div>{tab==="register"?<LoginCodes {...props}/>:<Gifts key={tab} {...props} mode={tab}/>}</section>
}
