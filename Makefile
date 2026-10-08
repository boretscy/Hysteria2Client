APP_NAME=SingBoxTray
APP_BUNDLE=bin/$(APP_NAME).app
DMG_NAME=SingBoxTray-1.0.0.dmg

all: build-app build-dmg

build-darwin:
	CGO_ENABLED=1 go build -o bin/$(APP_NAME) ./cmd/tray-client

build-app:
	@echo "==> Building $(APP_BUNDLE)..."
	rm -rf $(APP_BUNDLE)
	mkdir -p $(APP_BUNDLE)/Contents/MacOS
	mkdir -p $(APP_BUNDLE)/Contents/Resources
	cp assets/Info.plist $(APP_BUNDLE)/Contents/
	cp assets/AppIcon.icns $(APP_BUNDLE)/Contents/Resources/
	CGO_ENABLED=1 go build -o $(APP_BUNDLE)/Contents/MacOS/$(APP_NAME) ./cmd/tray-client
	chmod +x $(APP_BUNDLE)/Contents/MacOS/$(APP_NAME)
	@echo "==> $(APP_BUNDLE) successfully created."

build-dmg: build-app
	@echo "==> Building DMG package..."
	rm -rf bin/dmg_staging bin/$(DMG_NAME)
	mkdir -p bin/dmg_staging
	cp -R $(APP_BUNDLE) bin/dmg_staging/
	ln -s /Applications bin/dmg_staging/Applications
	hdiutil create -volname "SingBox Tray" -srcfolder bin/dmg_staging -ov -format UDZO bin/$(DMG_NAME)
	rm -rf bin/dmg_staging
	@echo "==> bin/$(DMG_NAME) successfully created."

test:
	go test -v -race ./...

lint:
	go vet ./...

clean:
	rm -rf bin
