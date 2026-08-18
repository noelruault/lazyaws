# Record only with a disposable or demo-safe profile because every captured frame is published.

Output docs/demo.gif

Set FontFamily "Menlo"
Set FontSize 15
Set Width 1010
Set Height 690
Set Theme "GitHub Dark"

Hide
Type "make build && AWS_PROFILE=demo ./lazyaws"
Enter
Sleep 4s
Show

Type "2"
Sleep 1.5s
Enter
Sleep 1.5s
Enter
Sleep 2s

Type ":ec2"
Sleep 1.5s
Enter
Sleep 2s

Type "A"
Sleep 1s
Type "which instances are running?"
Enter
Sleep 6s

Escape
Type "q"
