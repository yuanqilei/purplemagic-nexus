// Native D3D9 redemption overlay. No executable offsets, game memory writes,
// persistent tokens, browser component or game-thread network I/O.
#define WIN32_LEAN_AND_MEAN
#define NOMINMAX
#include <windows.h>
#include <d3d9.h>
#include <wincrypt.h>
#include <string>
#include <vector>
#include <mutex>
#include <thread>
#include <atomic>
#include <algorithm>
#include <climits>
#include <fstream>
#include "vendor/imgui/imgui.h"
#include "vendor/imgui/backends/imgui_impl_win32.h"
#include "vendor/imgui/backends/imgui_impl_dx9.h"
#include "vendor/minhook/include/MinHook.h"
#include "native_redemption.h"
extern IMGUI_IMPL_API LRESULT ImGui_ImplWin32_WndProcHandler(HWND,UINT,WPARAM,LPARAM);
namespace {
using EndSceneFn=HRESULT(WINAPI*)(IDirect3DDevice9*);
using ResetFn=HRESULT(WINAPI*)(IDirect3DDevice9*,D3DPRESENT_PARAMETERS*);
EndSceneFn originalEnd=nullptr; ResetFn originalReset=nullptr;
HWND game=nullptr; WNDPROC previousProc=nullptr; IDirect3DDevice9* activeDevice=nullptr;
std::recursive_mutex uiMutex; std::mutex resultMutex; std::atomic<bool> busy{false};
bool ready=false,opened=false; char code[512]={}; int selected=-1;
IDirect3DTexture9* panelTexture=nullptr;
struct Character {std::string id,name;}; std::vector<Character> characters;
std::string status;
HANDLE childInput=nullptr,childOutput=nullptr,childProcess=nullptr;
std::wstring Directory(){wchar_t p[32768];GetModuleFileNameW(nullptr,p,32768);std::wstring s(p);return s.substr(0,s.find_last_of(L"\\/"));}
std::string SessionToken(){std::wstring cmd=GetCommandLineW();auto pos=cmd.find(L"?de509f65");if(pos==std::wstring::npos||pos+33>=cmd.size()||cmd[pos+33]!=L'?')return {};std::string s;for(size_t i=pos+1;i<pos+33;i++){auto c=cmd[i];if(!((c>=L'0'&&c<=L'9')||(c>=L'a'&&c<=L'f')))return {};s+=(char)c;}return s;}
std::string Base64(const std::string& s){DWORD n=0;if(!CryptBinaryToStringA((const BYTE*)s.data(),(DWORD)s.size(),CRYPT_STRING_BASE64|CRYPT_STRING_NOCRLF,nullptr,&n))return {};std::string out(n,'\0');if(!CryptBinaryToStringA((const BYTE*)s.data(),(DWORD)s.size(),CRYPT_STRING_BASE64|CRYPT_STRING_NOCRLF,&out[0],&n))return {};while(!out.empty()&&out.back()==0)out.pop_back();return out;}
std::string Unbase64(const std::string& s){DWORD n=0;if(!CryptStringToBinaryA(s.c_str(),(DWORD)s.size(),CRYPT_STRING_BASE64,nullptr,&n,nullptr,nullptr)||n>4096)return {};std::string out(n,'\0');if(n&&!CryptStringToBinaryA(s.c_str(),(DWORD)s.size(),CRYPT_STRING_BASE64,(BYTE*)&out[0],&n,nullptr,nullptr))return {};return out;}
IDirect3DTexture9* LoadPanelTexture(IDirect3DDevice9* device){
 auto path=Directory()+L"\\redemption-bg.bmp";std::ifstream f(path,std::ios::binary);BITMAPFILEHEADER fh{};BITMAPINFOHEADER ih{};
 if(!f.read((char*)&fh,sizeof(fh))||!f.read((char*)&ih,sizeof(ih))||fh.bfType!=0x4D42||ih.biWidth<1||ih.biHeight<1||ih.biWidth>4096||ih.biHeight>4096||(ih.biBitCount!=24&&ih.biBitCount!=32)||ih.biCompression!=BI_RGB)return nullptr;
 IDirect3DTexture9* texture=nullptr;if(FAILED(device->CreateTexture(ih.biWidth,ih.biHeight,1,0,D3DFMT_A8R8G8B8,D3DPOOL_MANAGED,&texture,nullptr)))return nullptr;D3DLOCKED_RECT lock{};if(FAILED(texture->LockRect(0,&lock,nullptr,0))){texture->Release();return nullptr;}
 const int bpp=ih.biBitCount/8,rowBytes=((ih.biWidth*bpp+3)/4)*4;std::vector<BYTE> pixels((size_t)rowBytes*ih.biHeight);f.seekg(fh.bfOffBits);if(!f.read((char*)pixels.data(),(std::streamsize)pixels.size())){texture->UnlockRect(0);texture->Release();return nullptr;}
 for(int y=0;y<ih.biHeight;y++){auto* src=pixels.data()+(size_t)(ih.biHeight-1-y)*rowBytes;auto* dst=(BYTE*)lock.pBits+(size_t)y*lock.Pitch;for(int x=0;x<ih.biWidth;x++){dst[x*4]=src[x*bpp];dst[x*4+1]=src[x*bpp+1];dst[x*4+2]=src[x*bpp+2];dst[x*4+3]=255;}}
 texture->UnlockRect(0);return texture;
}
void CloseWorker(){if(childInput){CloseHandle(childInput);childInput=nullptr;}if(childOutput){CloseHandle(childOutput);childOutput=nullptr;}if(childProcess){if(WaitForSingleObject(childProcess,0)==WAIT_TIMEOUT)TerminateProcess(childProcess,1);CloseHandle(childProcess);childProcess=nullptr;}}
bool Write(const std::string& s){size_t offset=0;while(offset<s.size()){DWORD n=0;if(!WriteFile(childInput,s.data()+offset,(DWORD)(s.size()-offset),&n,nullptr)||!n)return false;offset+=n;}return true;}
bool StartWorker(){
 if(childProcess&&WaitForSingleObject(childProcess,0)==WAIT_TIMEOUT)return true;CloseWorker();
 auto token=SessionToken();if(token.empty())return false;
 SECURITY_ATTRIBUTES sa{sizeof(sa),nullptr,TRUE};HANDLE inRead=nullptr,outWrite=nullptr;
 if(!CreatePipe(&inRead,&childInput,&sa,0))return false;
 if(!CreatePipe(&childOutput,&outWrite,&sa,0)){CloseHandle(inRead);CloseWorker();return false;}
 SetHandleInformation(childInput,HANDLE_FLAG_INHERIT,0);SetHandleInformation(childOutput,HANDLE_FLAG_INHERIT,0);
 SIZE_T bytes=0;InitializeProcThreadAttributeList(nullptr,1,0,&bytes);std::vector<BYTE> attrs(bytes);
 STARTUPINFOEXW si{};si.StartupInfo.cb=sizeof(si);si.StartupInfo.dwFlags=STARTF_USESTDHANDLES;si.StartupInfo.hStdInput=inRead;si.StartupInfo.hStdOutput=outWrite;si.StartupInfo.hStdError=outWrite;si.lpAttributeList=(LPPROC_THREAD_ATTRIBUTE_LIST)attrs.data();
 HANDLE inherited[]={inRead,outWrite};bool attrReady=InitializeProcThreadAttributeList(si.lpAttributeList,1,0,&bytes)!=FALSE;
 bool configured=attrReady&&UpdateProcThreadAttribute(si.lpAttributeList,0,PROC_THREAD_ATTRIBUTE_HANDLE_LIST,inherited,sizeof(inherited),nullptr,nullptr)!=FALSE;
 PROCESS_INFORMATION pi{};auto dir=Directory(),exe=dir+L"\\PurpleMagic.Redeem.exe",cmd=L"\""+exe+L"\"";
 bool ok=configured&&CreateProcessW(exe.c_str(),&cmd[0],nullptr,nullptr,TRUE,CREATE_NO_WINDOW|EXTENDED_STARTUPINFO_PRESENT,nullptr,dir.c_str(),&si.StartupInfo,&pi)!=FALSE;
 if(attrReady)DeleteProcThreadAttributeList(si.lpAttributeList);CloseHandle(inRead);CloseHandle(outWrite);
 if(!ok){CloseWorker();return false;}childProcess=pi.hProcess;CloseHandle(pi.hThread);
 ok=Write(token+"\n");SecureZeroMemory(&token[0],token.size());if(!ok)CloseWorker();return ok;
}
bool ReadLine(std::string& line,ULONGLONG deadline){line.clear();while(GetTickCount64()<deadline){DWORD available=0;if(!PeekNamedPipe(childOutput,nullptr,0,nullptr,&available,nullptr))return false;if(!available){if(WaitForSingleObject(childProcess,0)!=WAIT_TIMEOUT)return false;Sleep(10);continue;}char c;DWORD n=0;if(!ReadFile(childOutput,&c,1,&n,nullptr)||n!=1)return false;if(c=='\n')return true;if(c!='\r')line+=c;if(line.size()>8192)return false;}return false;}
void Request(bool list,const std::string& id={},const std::string& input={}){
 if(busy.exchange(true))return;
#ifdef NATIVE_REDEMPTION_FIXTURE
 { std::lock_guard<std::mutex> lock(resultMutex);status=list?"隔离测试：未连接玩家数据库。":"隔离测试已接收输入，未发放奖励。"; }
 busy=false;return;
#endif
 {std::lock_guard<std::mutex> lock(resultMutex);status="正在兑换，请稍候…";}
 std::thread([input](){std::string message;bool ok=StartWorker();if(ok)ok=Write("redeem\t"+Base64(input)+"\n");
  std::string line;bool ended=false;auto deadline=GetTickCount64()+25000;
  while(ok&&ReadLine(line,deadline)){if(line=="END"){ended=true;break;}if(line.rfind("STATUS\t",0)==0)message=Unbase64(line.substr(7));}
  if(!ok||!ended){CloseWorker();message="请求未完成，请使用原码和原角色重试。";}
  {std::lock_guard<std::mutex> lock(resultMutex);status=message.empty()?"请求已完成。":message;}
  busy=false;
 }).detach();
}
LRESULT CALLBACK WindowProc(HWND h,UINT m,WPARAM w,LPARAM l){
 std::lock_guard<std::recursive_mutex> lock(uiMutex);
 if(ready){
 if(m==WM_KEYUP&&w==VK_F9){opened=!opened;return 0;}
  if(opened&&m==WM_KEYDOWN&&w==VK_ESCAPE){opened=false;return 0;}
  ImGui_ImplWin32_WndProcHandler(h,m,w,l);
  if(opened&&ImGui::GetIO().WantTextInput&&(m==WM_IME_COMPOSITION||m==WM_IME_STARTCOMPOSITION||m==WM_IME_ENDCOMPOSITION||m==WM_IME_CHAR))return DefWindowProcW(h,m,w,l);
  auto& io=ImGui::GetIO();
  if((io.WantCaptureMouse&&m>=WM_MOUSEFIRST&&m<=WM_MOUSELAST)||(opened&&io.WantCaptureKeyboard&&(m==WM_KEYDOWN||m==WM_KEYUP||m==WM_CHAR||m==WM_SYSKEYDOWN||m==WM_SYSKEYUP)))return 0;
 }
 return CallWindowProcW(previousProc,h,m,w,l);
}
void Initialize(IDirect3DDevice9* device){
 D3DDEVICE_CREATION_PARAMETERS params{};if(FAILED(device->GetCreationParameters(&params))||!params.hFocusWindow||!IsWindowVisible(params.hFocusWindow))return;
 DWORD pid=0;GetWindowThreadProcessId(params.hFocusWindow,&pid);if(pid!=GetCurrentProcessId())return;
 RECT rect{};GetClientRect(params.hFocusWindow,&rect);if(rect.right<640||rect.bottom<400)return;
 game=params.hFocusWindow;activeDevice=device;ImGui::CreateContext();auto& io=ImGui::GetIO();io.IniFilename=nullptr;io.LogFilename=nullptr;io.ConfigFlags|=ImGuiConfigFlags_NoMouseCursorChange;io.ConfigFlags|=ImGuiConfigFlags_NavEnableKeyboard;
 wchar_t win[32768];GetWindowsDirectoryW(win,32768);std::wstring font=std::wstring(win)+L"\\Fonts\\msyh.ttc";int n=WideCharToMultiByte(CP_UTF8,0,font.c_str(),-1,nullptr,0,nullptr,nullptr);std::string utf(n,0);WideCharToMultiByte(CP_UTF8,0,font.c_str(),-1,&utf[0],n,nullptr,nullptr);
 io.Fonts->TexDesiredWidth=2048;
 if(!io.Fonts->AddFontFromFileTTF(utf.c_str(),18.0f,nullptr,io.Fonts->GetGlyphRangesChineseFull()))io.Fonts->AddFontDefault();
 ImGui::StyleColorsDark();auto& style=ImGui::GetStyle();style.WindowRounding=6;style.FrameRounding=4;style.WindowPadding={20,18};style.ItemSpacing={10,12};style.Colors[ImGuiCol_WindowBg]={.075f,.065f,.055f,.98f};style.Colors[ImGuiCol_TitleBg]={.16f,.12f,.065f,1};style.Colors[ImGuiCol_TitleBgActive]={.24f,.17f,.075f,1};style.Colors[ImGuiCol_Button]={.34f,.24f,.11f,1};style.Colors[ImGuiCol_ButtonHovered]={.48f,.34f,.14f,1};style.Colors[ImGuiCol_ButtonActive]={.6f,.42f,.16f,1};style.Colors[ImGuiCol_CheckMark]={.95f,.75f,.32f,1};style.Colors[ImGuiCol_Text]={.94f,.87f,.72f,1};
 // Match the in-game coupon dialog's compact black-and-cyan frame and blue panel.
 style.WindowRounding=2;style.FrameRounding=2;style.WindowBorderSize=2;style.FrameBorderSize=1;style.WindowPadding={18,16};style.ItemSpacing={8,9};
 style.Colors[ImGuiCol_WindowBg]={.035f,.07f,.13f,.98f};style.Colors[ImGuiCol_Border]={.04f,.62f,.92f,1};style.Colors[ImGuiCol_TitleBg]={.015f,.08f,.13f,1};style.Colors[ImGuiCol_TitleBgActive]={.02f,.20f,.34f,1};style.Colors[ImGuiCol_FrameBg]={.005f,.07f,.11f,1};style.Colors[ImGuiCol_FrameBgHovered]={.01f,.14f,.22f,1};style.Colors[ImGuiCol_Button]={.02f,.20f,.35f,1};style.Colors[ImGuiCol_ButtonHovered]={.02f,.34f,.55f,1};style.Colors[ImGuiCol_ButtonActive]={.02f,.44f,.66f,1};style.Colors[ImGuiCol_CheckMark]={.2f,.83f,1,1};style.Colors[ImGuiCol_Text]={.94f,.97f,1,1};
 if(!ImGui_ImplWin32_Init(game)||!ImGui_ImplDX9_Init(device)){ImGui::DestroyContext();return;}
 SetLastError(0);previousProc=(WNDPROC)SetWindowLongPtrW(game,GWLP_WNDPROC,(LONG_PTR)WindowProc);
 if(!previousProc){ImGui_ImplDX9_Shutdown();ImGui_ImplWin32_Shutdown();ImGui::DestroyContext();return;}ready=true;
}
void Draw(){
 auto& io=ImGui::GetIO();ImGui_ImplDX9_NewFrame();ImGui_ImplWin32_NewFrame();ImGui::NewFrame();
 auto ini=Directory()+L"\\redemption.ini";static int x=GetPrivateProfileIntW(L"redemption",L"icon_x",INT_MIN,ini.c_str()),y=GetPrivateProfileIntW(L"redemption",L"icon_y",INT_MIN,ini.c_str());
 float px=x==INT_MIN?io.DisplaySize.x*.35f:(x<0?io.DisplaySize.x+x:(float)x),py=y==INT_MIN?io.DisplaySize.y*.685f:(y<0?io.DisplaySize.y+y:(float)y);px=std::max(0.f,std::min(px,io.DisplaySize.x-36));py=std::max(0.f,std::min(py,io.DisplaySize.y-36));
 ImGui::SetNextWindowPos({px,py},ImGuiCond_Always);ImGui::SetNextWindowBgAlpha(.9f);ImGui::PushStyleVar(ImGuiStyleVar_WindowPadding,{3,3});
 ImGui::Begin("##redemption-icon",nullptr,ImGuiWindowFlags_NoDecoration|ImGuiWindowFlags_AlwaysAutoResize|ImGuiWindowFlags_NoSavedSettings|ImGuiWindowFlags_NoNav);
 bool toggle=ImGui::InvisibleButton("##redeem-trigger",{30,30});auto icon=ImGui::GetItemRectMin();auto* iconDraw=ImGui::GetWindowDrawList();
 iconDraw->AddRectFilled(icon,ImVec2(icon.x+30,icon.y+30),IM_COL32(5,45,86,235),4.f);iconDraw->AddRect(icon,ImVec2(icon.x+30,icon.y+30),IM_COL32(120,220,255,255),4.f,0,1.5f);iconDraw->AddRect(ImVec2(icon.x+7,icon.y+8),ImVec2(icon.x+23,icon.y+22),IM_COL32(220,248,255,255),2.f,0,1.5f);iconDraw->AddLine(ImVec2(icon.x+10,icon.y+12),ImVec2(icon.x+20,icon.y+12),IM_COL32(220,248,255,255),1.f);iconDraw->AddLine(ImVec2(icon.x+10,icon.y+16),ImVec2(icon.x+18,icon.y+16),IM_COL32(220,248,255,255),1.f);
 if(toggle)opened=!opened;if(ImGui::IsItemHovered())ImGui::SetTooltip("口令码 / CDK（F9）");ImGui::End();ImGui::PopStyleVar();
 if(opened){
  ImGui::SetNextWindowSize({350,275},ImGuiCond_FirstUseEver);ImGui::SetNextWindowPos({io.DisplaySize.x/2,io.DisplaySize.y/2},ImGuiCond_FirstUseEver,{.5f,.5f});
  if(ImGui::Begin("口令码",&opened,ImGuiWindowFlags_NoCollapse|ImGuiWindowFlags_NoResize|ImGuiWindowFlags_NoSavedSettings)){
   auto* draw=ImGui::GetWindowDrawList();auto wp=ImGui::GetWindowPos();auto ws=ImGui::GetWindowSize();
   if(!panelTexture)panelTexture=LoadPanelTexture(activeDevice);
   if(panelTexture){auto a=ImVec2(wp.x+2,wp.y+ImGui::GetFrameHeight()+2),b=ImVec2(wp.x+ws.x-2,wp.y+ws.y-2);draw->AddImage((ImTextureID)panelTexture,a,b);}
   auto centered=[](const char* text){float width=ImGui::CalcTextSize(text).x;ImGui::SetCursorPosX(std::max(0.f,(ImGui::GetWindowWidth()-width)*.5f));ImGui::TextUnformatted(text);};
   ImGui::SetWindowFontScale(1.18f);centered("口令码");ImGui::SetWindowFontScale(1.0f);centered("输入口令码可以获得奖励！");centered("请输入口令码/CDK");
   ImGui::BeginDisabled(busy.load());
   float inputWidth=250.f;ImGui::SetCursorPosX((ImGui::GetWindowWidth()-inputWidth)*.5f);ImGui::SetNextItemWidth(inputWidth);ImGui::InputTextWithHint("##redeem-code","",code,sizeof(code));
   ImGui::Spacing();if(!status.empty()){auto msg=status;float tw=ImGui::CalcTextSize(msg.c_str()).x;ImGui::SetCursorPosX(std::max(0.f,(ImGui::GetWindowWidth()-tw)*.5f));ImGui::TextUnformatted(msg.c_str());}ImGui::Spacing();ImGui::BeginDisabled(!code[0]||strlen(code)>128);
   float buttonWidth=115.f,gap=12.f;ImGui::SetCursorPosX((ImGui::GetWindowWidth()-buttonWidth*2-gap)*.5f);
   if(ImGui::Button("确认",{buttonWidth,34})){Request(false,"",code);}ImGui::SameLine(0,gap);if(ImGui::Button("取消",{buttonWidth,34}))opened=false;ImGui::EndDisabled();ImGui::EndDisabled();
  }ImGui::End();
 }
 io.MouseDrawCursor=io.WantCaptureMouse;ImGui::EndFrame();ImGui::Render();ImGui_ImplDX9_RenderDrawData(ImGui::GetDrawData());
}
HRESULT WINAPI EndScene(IDirect3DDevice9* device){
 {std::lock_guard<std::recursive_mutex> lock(uiMutex);try{if(!ready)Initialize(device);if(ready&&device==activeDevice)Draw();}catch(...){opened=false;}}
 return originalEnd(device);
}
HRESULT WINAPI Reset(IDirect3DDevice9* device,D3DPRESENT_PARAMETERS* parameters){
 std::lock_guard<std::recursive_mutex> lock(uiMutex);if(ready&&device==activeDevice)ImGui_ImplDX9_InvalidateDeviceObjects();auto hr=originalReset(device,parameters);if(SUCCEEDED(hr)&&ready&&device==activeDevice)ImGui_ImplDX9_CreateDeviceObjects();return hr;
}
DWORD WINAPI Install(LPVOID){
 for(int i=0;i<120&&!GetModuleHandleW(L"d3d9.dll");i++)Sleep(1000);if(!GetModuleHandleW(L"d3d9.dll"))return 0;
 auto d3d=Direct3DCreate9(D3D_SDK_VERSION);if(!d3d)return 0;
 WNDCLASSW wc{};wc.lpfnWndProc=DefWindowProcW;wc.hInstance=GetModuleHandleW(nullptr);wc.lpszClassName=L"PurpleMagicRedemptionProbe";RegisterClassW(&wc);
 auto window=CreateWindowW(wc.lpszClassName,L"",WS_OVERLAPPED,0,0,8,8,nullptr,nullptr,wc.hInstance,nullptr);D3DPRESENT_PARAMETERS pp{};pp.Windowed=TRUE;pp.SwapEffect=D3DSWAPEFFECT_DISCARD;pp.hDeviceWindow=window;IDirect3DDevice9* device=nullptr;
 auto hr=d3d->CreateDevice(D3DADAPTER_DEFAULT,D3DDEVTYPE_HAL,window,D3DCREATE_SOFTWARE_VERTEXPROCESSING,&pp,&device);
 if(SUCCEEDED(hr)){auto table=*reinterpret_cast<void***>(device);if(MH_Initialize()==MH_OK){if(MH_CreateHook(table[42],EndScene,reinterpret_cast<void**>(&originalEnd))==MH_OK&&MH_CreateHook(table[16],Reset,reinterpret_cast<void**>(&originalReset))==MH_OK){MH_QueueEnableHook(table[42]);MH_QueueEnableHook(table[16]);MH_ApplyQueued();}}device->Release();}
 d3d->Release();if(window)DestroyWindow(window);UnregisterClassW(wc.lpszClassName,wc.hInstance);return 0;
}
}
void StartNativeRedemption(){auto thread=CreateThread(nullptr,0,Install,nullptr,0,nullptr);if(thread)CloseHandle(thread);}

#ifdef NATIVE_REDEMPTION_FIXTURE
void NativeRedemptionFixtureRender(IDirect3DDevice9* d){std::lock_guard<std::recursive_mutex> lock(uiMutex);if(!ready)Initialize(d);if(ready)Draw();}
void NativeRedemptionFixtureReset(){if(ready)ImGui_ImplDX9_InvalidateDeviceObjects();}
#endif
