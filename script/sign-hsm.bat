@echo off

if "%DLIB_PATH%" == "" (
    echo skipping Windows code-signing; DLIB_PATH not set
    exit /b
)

if "%METADATA_PATH%" == "" (
    echo skipping Windows code-signing; METADATA_PATH not set
    exit /b
)

REM For more information on signtool, see https://learn.microsoft.com/en-us/windows/win32/seccrypto/signtool
"C:\Program Files (x86)\Windows Kits\10\bin\10.0.22621.0\x64\signtool" sign /d "GitHub CLI" /fd sha256 /td sha256 /tr http://timestamp.acs.microsoft.com /v /dlib "%DLIB_PATH%" /dmdf "%METADATA_PATH%" "%1"
