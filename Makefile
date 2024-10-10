all:
	go build -trimpath -buildmode=c-shared -o build/out_lm.so ./output

clean:
	rm -rf *.so *.h build/

windows:
	# Building modules for windows from macOS with CGO enabled requires cross dedicated compiler, e.g: mingw-w64 toolchain
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc go build -trimpath -buildmode=c-shared -o build/out_lm-windows.so ./output

linux-amd:
	# Building modules for linux from macOS with CGO enabled requires dedicated cross compiler, e.g:
	# brew tap messense/macos-cross-toolchains
	# brew install x86_64-unknown-linux-gnu
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 CC=x86_64-linux-gnu-gcc go build -trimpath -buildmode=c-shared -o build/out_lm-linux.so ./output

linux-arm:
	# brew install aarch64-unknown-linux-gnu
	CGO_ENABLED=1 GOOS=linux GOARCH=arm64 CC=aarch64-linux-gnu-gcc go build -trimpath -buildmode=c-shared -o build/out_lm-linux-arm64.so ./output

darwin:
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 CC=clang go build -trimpath -buildmode=c-shared -o build/out_lm-macOS.so ./output

darwin-arm:
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -trimpath -buildmode=c-shared -o build/out_lm-macOS-arm64.so ./output