
#### macOS Desktop 
Building the standalone web-server binary for macOS is just as easy as Linux and Windows. You don't need a Mac to do it; your Linux Docker container can cross-compile it perfectly.

Because Apple transitioned from Intel to their own Apple Silicon (M1/M2/M3) chips, you usually compile two versions:

For older Intel Macs:
```
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o bin/omn-go-mac-intel main_desktop.go
```

For newer Apple Silicon Macs:
```
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o bin/omn-go-mac-arm main_desktop.go
```
Note: Because you are cross-compiling from Linux, the binary won't have an "Apple Developer Signature." When a Mac user tries to run it, Apple's "Gatekeeper" will block it by default. The user will have to manually bypass it by going to System Settings -> Privacy & Security -> "Open Anyway".
