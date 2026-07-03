; Inno Setup script for the AI Terminal Gateway.
; Builds a single Setup.exe wizard. Per-user install (no admin required):
; installs the prebuilt binary + scripts, lets the user pick a provider, then
; runs setup.ps1 (generates .env with a key, auto-detects Docker, registers the
; logon auto-start task). Build with: scripts\build-installer.ps1

#define MyAppName "AI Terminal Gateway"
#define MyAppVersion "0.1.0"
#define MyAppPublisher "stonehappi"
#define MyAppURL "https://github.com/stonehappi/ai-terminal-gateway"

[Setup]
AppId={{7E2C1F9A-3B4D-4E6A-9C1B-AITERMGATEWAY01}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
DefaultDirName={localappdata}\AI Terminal Gateway
DefaultGroupName={#MyAppName}
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
OutputDir=Output
OutputBaseFilename=ai-terminal-gateway-setup
Compression=lzma
SolidCompression=yes
WizardStyle=modern
ArchitecturesInstallIn64BitMode=x64compatible

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "loginnow"; Description: "Log in to my AI provider now (opens a browser)"
Name: "startnow"; Description: "Start the gateway now (otherwise it starts at next logon)"

[Files]
Source: "..\ai-gateway-api.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\scripts\*"; DestDir: "{app}\scripts"; Flags: ignoreversion recursesubdirs
Source: "..\README.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\.env.example"; DestDir: "{app}"; Flags: ignoreversion

[Run]
; Configure everything after files are copied. Not hidden, so the user sees the
; summary (URL + API key) and any provider-login browser prompt.
Filename: "powershell.exe"; Parameters: "{code:GetSetupArgs}"; \
  WorkingDir: "{app}"; StatusMsg: "Configuring the gateway..."; Flags: waituntilterminated

[UninstallRun]
; Remove the auto-start task on uninstall.
Filename: "powershell.exe"; \
  Parameters: "-NoProfile -ExecutionPolicy Bypass -File ""{app}\scripts\uninstall-autostart.ps1"""; \
  Flags: runhidden; RunOnceId: "RemoveAutostart"

[Code]
var
  ProviderPage: TInputOptionWizardPage;

procedure InitializeWizard();
begin
  ProviderPage := CreateInputOptionPage(wpSelectTasks,
    'AI Provider', 'Which AI CLI should the gateway use by default?',
    'Pick the tool you have an account for. You can change this later in .env, ' +
    'and callers can override it per request. The chosen CLI must be installed ' +
    'and logged in.',
    True, False);
  ProviderPage.Add('OpenAI Codex  (codex)');
  ProviderPage.Add('Claude Code   (claude)');
  ProviderPage.Add('agy           (agy)');
  ProviderPage.SelectedValueIndex := 0;
end;

function GetProvider(): String;
begin
  case ProviderPage.SelectedValueIndex of
    1: Result := 'claude';
    2: Result := 'agy';
  else
    Result := 'codex';
  end;
end;

// Build the full powershell command line for the [Run] step.
function GetSetupArgs(Param: String): String;
var
  Args: String;
begin
  Args := '-NoProfile -ExecutionPolicy Bypass -File "' + ExpandConstant('{app}\scripts\setup.ps1') + '"';
  Args := Args + ' -Provider ' + GetProvider();
  if WizardIsTaskSelected('loginnow') then
    Args := Args + ' -LoginNow';
  if WizardIsTaskSelected('startnow') then
    Args := Args + ' -StartNow';
  Result := Args;
end;
