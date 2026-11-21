using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using System.Text.RegularExpressions;
using ServerS5A3;

static class Program
{
    static string B64(string s) => Convert.ToBase64String(Encoding.UTF8.GetBytes(s));
    static void Status(string s) => Console.WriteLine("STATUS\t" + B64(s));
    static PlayerProfile Profile(string path) {
        using var d = JsonDocument.Parse(File.ReadAllText(path)); var r=d.RootElement;
        string S(string k) => r.TryGetProperty(k,out var v) ? v.GetString() ?? "" : "";
        return new(S("ApiUrl"),S("CertificateSha256"),S("UpdatePublicKey"),S("SettingsUrl"),S("GameHost"),r.GetProperty("BootstrapPort").GetInt32(),S("ExeSha256"),S("PvfSha256"),"");
    }
    static async Task<int> Main(string[] args) {
        Console.InputEncoding=Encoding.UTF8;Console.OutputEncoding=new UTF8Encoding(false);
        if(args.Length>0 && args[0]=="--selftest") {
            var sample="开服快乐123"; if(Encoding.UTF8.GetString(Convert.FromBase64String(B64(sample)))!=sample) return 1;
            Console.WriteLine("PASS unicode protocol roundtrip");return 0;
        }
        try {
            var path=Path.Combine(AppContext.BaseDirectory,"PurpleMagic.profile.json");
            var profile=Profile(path);
            if(!Uri.TryCreate(profile.ApiUrl,UriKind.Absolute,out var origin)||origin.Scheme!="https"||origin.AbsolutePath!="/"||!Regex.IsMatch(profile.CertificateSha256,"^[a-fA-F0-9]{64}$")) throw new InvalidDataException();
            if(args.Length>0 && args[0]=="--check-server") { using var check=new PlayerApi(profile);await check.CheckStatus();Console.WriteLine("PASS pinned TLS status");return 0; }
            var token=Console.ReadLine()??"";
            if(!Regex.IsMatch(token,"^de509f65[a-f0-9]{24}$")) { Status("请使用配套启动器重新进入游戏。");Console.WriteLine("END");return 2; }
            using var api=new PlayerApi(profile);
            while(Console.ReadLine() is {} line) {
                try {
                    if(line.Length>4096) throw new InvalidDataException();
                    var fields=line.Split('\t');
                    var chars=false;
                    if(fields.Length!=2||fields[0]!="redeem")throw new InvalidDataException();
                    var characterId="";
                    if(!chars){
                        using var characterRequest=new HttpRequestMessage(HttpMethod.Get,profile.ApiUrl+"/api/gifts/characters");
                        characterRequest.Headers.Authorization=new AuthenticationHeaderValue("Bearer",token);
                        using var characterResponse=await api.Http.SendAsync(characterRequest,HttpCompletionOption.ResponseHeadersRead);
                        if(!characterResponse.IsSuccessStatusCode)throw new HttpRequestException();
                        var characterJson=await characterResponse.Content.ReadAsStringAsync();using var characterDoc=JsonDocument.Parse(characterJson);
                        foreach(var c in characterDoc.RootElement.GetProperty("data").EnumerateArray()){var id=c.GetProperty("id").GetString()??"";if(Regex.IsMatch(id,"^[0-9]{1,20}$")){characterId=id;break;}}
                        if(characterId.Length==0){Status("当前账号没有可用角色。");Console.WriteLine("END");continue;}
                    }
                    using var request=new HttpRequestMessage(HttpMethod.Post,profile.ApiUrl+"/api/gifts/redeem");
                    request.Headers.Authorization=new AuthenticationHeaderValue("Bearer",token);
                    if(!chars) {
                        var code=Encoding.UTF8.GetString(Convert.FromBase64String(fields[1]));
                        if(string.IsNullOrWhiteSpace(code)||Encoding.UTF8.GetByteCount(code)>128)throw new InvalidDataException();
                        using var body=new MemoryStream();using(var w=new Utf8JsonWriter(body)){w.WriteStartObject();w.WriteString("code",code);w.WriteString("characterId",characterId);w.WriteEndObject();}
                        request.Content=new ByteArrayContent(body.ToArray());request.Content.Headers.ContentType=new MediaTypeHeaderValue("application/json");
                    }
                    using var response=await api.Http.SendAsync(request,HttpCompletionOption.ResponseHeadersRead);
                    await using var stream=await response.Content.ReadAsStreamAsync();using var buffer=new MemoryStream();var chunk=new byte[4096];int n;
                    while((n=await stream.ReadAsync(chunk))>0){if(buffer.Length+n>65536)throw new InvalidDataException();buffer.Write(chunk,0,n);}
                    var raw=Encoding.UTF8.GetString(buffer.ToArray());
                    if(!response.IsSuccessStatusCode){Status(response.StatusCode==System.Net.HttpStatusCode.Unauthorized?"登录凭据已过期，请重新进入游戏。":raw.Length<=200?raw.Trim():"兑换服务暂不可用，请稍后重试。");}
                    else {
                        using var doc=JsonDocument.Parse(raw);var data=doc.RootElement.GetProperty("data");
                        var delivered=data.GetProperty("status").GetString()=="delivered";Status(delivered?"兑换成功，奖励已通过游戏邮件发放。":"发放待确认，请使用原码和当前账号任一角色重试。");
                    }
                } catch(HttpRequestException){Status("连接或证书校验失败，请检查服务器配置后重试。");}
                catch(TaskCanceledException){Status("请求超时，请使用原码和原角色重试。");}
                catch(Exception){Status("兑换请求未完成，请核对输入或稍后重试。");}
                Console.WriteLine("END");Console.Out.Flush();
            }
            return 0;
        } catch(Exception) {Status("兑换配置不可用，请检查配套的 PurpleMagic.profile.json。");Console.WriteLine("END");return 1;}
    }
}
