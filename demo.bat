@echo off
chcp 65001 >nul
echo ==================================================
echo   New API 闲置 API Key 共享分成 - 答辩演示模拟器
echo ==================================================
echo 正在模拟消费者发起 API 调用并扣费结算...
"D:\Go\go\bin\go.exe" run scripts/demo_simulate.go
pause
