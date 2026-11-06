# Runs the real hck inside the vhs container. The chart is created off screen
# with "hck new payments-api", so the recording starts on an existing chart.
# To regenerate, from the repo root:
#   GOOS=linux go build -o bin/linux/hck ./cmd
#   docker run --rm -v "$PWD:/vhs" ghcr.io/charmbracelet/vhs examples/demo/demo.tape
Output examples/demo/demo.gif

Set Shell bash
Set FontSize 16
Set Width 1280
Set Height 620
Set Padding 16
Set TypingSpeed 35ms
Set Theme "Catppuccin Mocha"

Hide
Type "export PATH=/vhs/bin/linux:$PATH && mkdir -p /tmp/charts && cd /tmp/charts && hck new payments-api >/dev/null && cd payments-api"
Enter
Type "PS1='$ ' && clear"
Enter
Sleep 500ms
Show

Type "ls"
Enter
Sleep 1.5s
Type "cp values.yaml /tmp/before.yaml"
Enter
Sleep 500ms
Type "hck add servicemonitor"
Enter
Sleep 4s
Type "diff /tmp/before.yaml values.yaml | head -10"
Enter
Sleep 5s
