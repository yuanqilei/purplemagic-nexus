using System.Runtime.InteropServices;
using System.Security.Cryptography;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace Dnf90.Launcher;

internal sealed record SavedCredentials(string Account, string Password);

[JsonSerializable(typeof(SavedCredentials))]
internal partial class CredentialJson : JsonSerializerContext { }

internal static class CredentialVault
{
    [StructLayout(LayoutKind.Sequential)]
    private struct DataBlob { public int Length; public IntPtr Data; }
    [DllImport("crypt32.dll", SetLastError=true, CharSet=CharSet.Unicode)]
    private static extern bool CryptProtectData(ref DataBlob input, string? description, IntPtr entropy, IntPtr reserved, IntPtr prompt, uint flags, out DataBlob output);
    [DllImport("crypt32.dll", SetLastError=true)]
    private static extern bool CryptUnprotectData(ref DataBlob input, IntPtr description, IntPtr entropy, IntPtr reserved, IntPtr prompt, uint flags, out DataBlob output);
    [DllImport("kernel32.dll")] private static extern IntPtr LocalFree(IntPtr memory);
    private static string PathFor(string root) => Path.Combine(root,"credentials.dpapi");

    public static void Save(string root, string account, string password)
    {
        if (!OperatingSystem.IsWindows()) throw new PlatformNotSupportedException();
        var plain=JsonSerializer.SerializeToUtf8Bytes(new SavedCredentials(account,password),CredentialJson.Default.SavedCredentials);
        var input=Blob(plain); DataBlob output=default;
        try {
            if(!CryptProtectData(ref input,"PurpleMagic launcher credentials",IntPtr.Zero,IntPtr.Zero,IntPtr.Zero,1,out output)) throw new CryptographicException(Marshal.GetLastWin32Error());
            var encrypted=new byte[output.Length]; Marshal.Copy(output.Data,encrypted,0,encrypted.Length);
            SafeFiles.Atomic(PathFor(root),encrypted);
        } finally { Free(input); if(output.Data!=IntPtr.Zero) LocalFree(output.Data); CryptographicOperations.ZeroMemory(plain); }
    }
    public static SavedCredentials? Load(string root)
    {
        if (!OperatingSystem.IsWindows()) return null;
        var path=PathFor(root); SafeFiles.NoLinks(path); if(!File.Exists(path)) return null;
        var encrypted=File.ReadAllBytes(path); if(encrypted.Length is <1 or >4096) return null;
        var input=Blob(encrypted); DataBlob output=default;
        try {
            if(!CryptUnprotectData(ref input,IntPtr.Zero,IntPtr.Zero,IntPtr.Zero,IntPtr.Zero,1,out output)) return null;
            var plain=new byte[output.Length]; Marshal.Copy(output.Data,plain,0,plain.Length);
            try {
                var saved=JsonSerializer.Deserialize(plain,CredentialJson.Default.SavedCredentials);
                return saved is { Account.Length: >0 and <=64, Password.Length: >0 and <=64 } ? saved : null;
            }
            finally { CryptographicOperations.ZeroMemory(plain); }
        } catch (JsonException) { return null; }
        finally { Free(input); if(output.Data!=IntPtr.Zero) { Marshal.Copy(new byte[output.Length],0,output.Data,output.Length); LocalFree(output.Data); } CryptographicOperations.ZeroMemory(encrypted); }
    }
    public static void Clear(string root)
    {
        var path=PathFor(root); SafeFiles.NoLinks(path); if(File.Exists(path)) File.Delete(path);
    }
    private static DataBlob Blob(byte[] bytes) { var p=Marshal.AllocHGlobal(bytes.Length); Marshal.Copy(bytes,0,p,bytes.Length); return new DataBlob{Length=bytes.Length,Data=p}; }
    private static void Free(DataBlob blob) { if(blob.Data!=IntPtr.Zero){ Marshal.Copy(new byte[blob.Length],0,blob.Data,blob.Length); Marshal.FreeHGlobal(blob.Data); } }
}
