# scripts/vendor/

LhmHelper 패키지 빌드 시 필요한 외부 바이너리를 수동 다운로드하여 이 디렉토리에 배치합니다. git에 커밋되지 않습니다 (`.gitignore` 참고).

## 필요 파일

### `NDP48-x86-x64-AllOS-ENU.exe`

.NET Framework 4.8 오프라인 설치기. `install_ResourceAgent.bat`이 Windows PC에서 .NET Framework 4.8 미설치 시 자동으로 실행합니다.

- **파일명**: `NDP48-x86-x64-AllOS-ENU.exe`
- **크기**: 약 111.94 MB
- **공식 다운로드**: https://dotnet.microsoft.com/download/dotnet-framework/net48
  - "Offline installer" 버튼 클릭
- **직접 링크**: https://download.microsoft.com/download/f/3/a/f3a6af84-da23-40a5-8d1c-49cc10c8e76f/NDP48-x86-x64-AllOS-ENU.exe
- **지원 OS**: Windows 7 SP1, 8.1, 10, 11, Server 2008 R2 SP1, 2012 R2, 2016, 2019, 2022

## 체크섬 (참고)

파일 무결성 검증용 SHA-256 (Microsoft 공식 값과 일치해야 함):

```
SHA-256: 95889d6de3f2070c07790ad6cf2000d33d9a1bdfc6a381725ab82ab1c314fd53
```

macOS / Linux:
```bash
shasum -a 256 NDP48-x86-x64-AllOS-ENU.exe
```

Windows PowerShell:
```powershell
Get-FileHash NDP48-x86-x64-AllOS-ENU.exe -Algorithm SHA256
```

## 회사 내부 미러링

MS 공식 URL이 사라질 리스크 대비 **회사 내부 파일 서버에 동일한 파일 보관 권장**. 팀별 구체적 미러 URL은 여기에 추가.

- 예: `\\fileserver\shared\vendors\NDP48-x86-x64-AllOS-ENU.exe`

## 사용

파일 배치 후 `./scripts/package.sh --build --lhmhelper` 실행 시 자동으로 패키지에 포함됩니다. 누락 시 빌드 에러.
