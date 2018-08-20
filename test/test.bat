@echo off 
setlocal enabledelayedexpansion
set SLEEP=ping 127.0.0.1 -n
echo go build ...
go build -race ..\hgrid\main.go
set count=%1
for /L %%i in (1,1,%count%) do rmdir /S/Q node%%i
for /L %%i in (1,1,%count%) do md node%%i
for /L %%i in (1,1,%count%) do copy .\main.exe .\node%%i\ > %temp%\null
for /L %%i in (1,1,%count%) do echo { >> .\node%%i\config.json
for /L %%i in (1,1,%count%) do set /a port=33200+%%i & echo "addr":"127.0.0.1:!port!", >> .\node%%i\config.json
for /L %%i in (1,1,%count%) do echo "coinbase":[%%i,%%i,%%i,%%i,%%i,%%i,%%i,%%i,%%i,%%i,%%i,%%i,%%i,%%i,%%i,%%i,%%i,%%i,%%i,%%i], >> .\node%%i\config.json
set peers="peers":[
for /L %%i in (1,1,%count%) do set /a port=33200+%%i & set peers=!peers!"127.0.0.1:!port!",
set peers=%peers:~0,-1%
set peers=%peers%],
for /L %%i in (1,1,%count%) do echo %peers% >> .\node%%i\config.json
for /L %%i in (1,1,%count%) do echo "interMilli":500 >> .\node%%i\config.json
for /L %%i in (1,1,%count%) do echo } >> .\node%%i\config.json
echo Start
for /L %%i in (1,1,%count%) do echo node_%%i ... & start cmd /c "cd node%%i&main.exe&pause" & %SLEEP% 0 > %temp%\null
echo done
