# common.ps1 — 发行包脚本公共逻辑（PS 5.1 兼容）。
# 读取托盘向导写入的应用配置；敏感字段为 DPAPI 密文，在此解密到进程内存，不落日志。

function Get-AppConfig {
    $cfgPath = Join-Path $env:LOCALAPPDATA "Hector ResearchOS\config.json"
    if (-not (Test-Path $cfgPath)) {
        throw "未找到应用配置 $cfgPath —— 请先运行 researchos-tray.exe 完成首次初始化"
    }
    $cfg = Get-Content $cfgPath -Raw -Encoding UTF8 | ConvertFrom-Json
    if (-not $cfg.initialized) { throw "应用尚未完成初始化（先运行 researchos-tray.exe）" }

    # DPAPI 解密（与 Go CryptProtectData 兼容）
    Add-Type -AssemblyName System.Security
    foreach ($field in @("pgDsnEnc", "apiTokenEnc", "secretKeyEnc")) {
        $v = $cfg.$field
        if ($v -and $v.StartsWith("dpapi:")) {
            $blob = [Convert]::FromBase64String($v.Substring(6))
            $plain = [System.Security.Cryptography.ProtectedData]::Unprotect($blob, $null, [System.Security.Cryptography.DataProtectionScope]::CurrentUser)
            $cfg | Add-Member -NotePropertyName $field.Replace("Enc", "") -NotePropertyValue ([Text.Encoding]::UTF8.GetString($plain)) -Force
        }
    }
    return $cfg
}

function Get-BackendExe {
    $p = Join-Path $PSScriptRoot "..\bin\researchos.exe"
    if (-not (Test-Path $p)) { throw "未找到后端程序 $p" }
    return (Resolve-Path $p).Path
}

function Test-BackendAlive {
    param([int]$PidToCheck)
    try {
        $proc = Get-Process -Id $PidToCheck -ErrorAction Stop
        # 身份确认：进程名必须是 researchos（防止 PID 复用误判）
        return ($proc.ProcessName -ieq "researchos")
    } catch { return $false }
}

function Wait-Healthy {
    param([string]$Addr, [int]$Seconds = 20)
    for ($i = 0; $i -lt ($Seconds * 2); $i++) {
        try {
            $r = Invoke-WebRequest -Uri "http://$Addr/api/v1/health" -UseBasicParsing -TimeoutSec 2
            if ($r.StatusCode -eq 200) { return $true }
        } catch { Start-Sleep -Milliseconds 500 }
    }
    return $false
}

function Remove-DuplicateCaseEnvVars {
    # 大小写重复的环境变量（如 HTTPS_PROXY 与 https_proxy 并存）会让 .NET
    # Start-Process 构建环境字典时抛"已添加项"异常。仅清理当前进程作用域。
    $seen = @{}
    $toRemove = @()
    foreach ($pair in ([System.Environment]::GetEnvironmentVariables("Process").GetEnumerator())) {
        $key = $pair.Key.ToLower()
        if ($seen.ContainsKey($key)) { $toRemove += $pair.Key }
        else { $seen[$key] = $true }
    }
    foreach ($name in $toRemove) {
        [System.Environment]::SetEnvironmentVariable($name, $null, "Process")
    }
}
