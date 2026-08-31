Output test-artifacts/cli-demo.gif

Require task
Require docker

Set Shell "zsh"
Set FontSize 20
Set Width 1200
Set Height 480
Set Theme "rose-pine-moon"
Set Padding 24
Set TypingSpeed 45ms
Set Framerate 30

Hide
Type "export PS1='%1~ %# '"
Enter
Type "task dev >/dev/null 2>&1 &"
Enter
Sleep 7s
Type "clear"
Enter
Show

Type "task run -- --image alpine echo hello"
Enter
Sleep 5s

Hide
Type "kill %1"
Enter
Type "wait %1"
Enter
Sleep 4s
