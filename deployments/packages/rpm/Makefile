PACKAGE_NAME := gpuhealth
VERSION := 0.1.0
RELEASE := 1

# RPM build directories
RPMBUILD_DIR := $(CURDIR)
SOURCES_DIR := $(RPMBUILD_DIR)/SOURCES
SPECS_DIR := $(RPMBUILD_DIR)/SPECS
RPMS_DIR := $(RPMBUILD_DIR)/RPMS
SRPMS_DIR := $(RPMBUILD_DIR)/SRPMS

# Optional extra flags to pass to rpmbuild (e.g., --nodeps)
RPMBUILD_FLAGS ?=

# Source archive
SOURCE_ARCHIVE := $(PACKAGE_NAME)-$(VERSION).tar.gz

.PHONY: all clean prep build-rpm install-deps setup-dirs

all: build-rpm

# Install RPM build dependencies
install-deps:
	@echo "Installing RPM build dependencies..."
	sudo dnf install -y rpm-build rpm-devel rpmlint rpmdevtools golang make || \
	sudo yum install -y rpm-build rpm-devel rpmlint rpmdevtools golang make || \
	sudo zypper install -y rpm-build rpm-devel rpmlint rpmdevtools golang make || \
	sudo apt-get update && sudo apt-get install -y rpm build-essential golang-go make || \
	echo "Please install rpm-build, golang, make, and optionally rpmlint manually"

# Setup RPM build directory structure
setup-dirs:
	@echo "Setting up RPM build directories..."
	mkdir -p $(SOURCES_DIR) $(SPECS_DIR) $(RPMS_DIR) $(SRPMS_DIR) $(RPMBUILD_DIR)/BUILD

# Prepare source archive
prep: setup-dirs
	@echo "Preparing source archive..."
	cd ../../.. && \
	tar --exclude='.git' \
	    --exclude='bin' \
	    --exclude='deployments/packages/debian/gpuhealth' \
	    --exclude='deployments/packages/rpm/$(SOURCE_ARCHIVE)' \
	    --transform 's,^,$(PACKAGE_NAME)-$(VERSION)/,' \
	    -czf deployments/packages/rpm/$(SOURCE_ARCHIVE) .
	cp $(SOURCE_ARCHIVE) $(SOURCES_DIR)/
	cp $(PACKAGE_NAME).spec $(SPECS_DIR)/

# Build RPM package
build-rpm: prep
	@echo "Building RPM package..."
	rpmbuild --define '_topdir $(RPMBUILD_DIR)' $(RPMBUILD_FLAGS) -ba $(SPECS_DIR)/$(PACKAGE_NAME).spec
	@echo ""
	@echo "RPM packages built successfully:"
	@echo "Binary RPM: $(RPMS_DIR)/*/$(PACKAGE_NAME)-$(VERSION)-$(RELEASE).*.rpm"
	@echo "Source RPM: $(SRPMS_DIR)/$(PACKAGE_NAME)-$(VERSION)-$(RELEASE).*.src.rpm"

# Build only binary RPM
build-binary-rpm: prep
	@echo "Building binary RPM package..."
	rpmbuild --define '_topdir $(RPMBUILD_DIR)' $(RPMBUILD_FLAGS) -bb $(SPECS_DIR)/$(PACKAGE_NAME).spec

# Build only source RPM
build-source-rpm: prep
	@echo "Building source RPM package..."
	rpmbuild --define '_topdir $(RPMBUILD_DIR)' $(RPMBUILD_FLAGS) -bs $(SPECS_DIR)/$(PACKAGE_NAME).spec

# Check RPM with rpmlint
check: build-rpm
	@echo "Checking RPM package with rpmlint..."
	rpmlint $(RPMS_DIR)/*/$(PACKAGE_NAME)-$(VERSION)-$(RELEASE).*.rpm || true
	rpmlint $(SRPMS_DIR)/$(PACKAGE_NAME)-$(VERSION)-$(RELEASE).*.src.rpm || true

# Install the built RPM
install: build-rpm
	@echo "Installing RPM package..."
	sudo rpm -Uvh $(RPMS_DIR)/*/$(PACKAGE_NAME)-$(VERSION)-$(RELEASE).*.rpm

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -f $(SOURCE_ARCHIVE)
	rm -rf $(RPMBUILD_DIR)/BUILD/$(PACKAGE_NAME)-$(VERSION) || true
	rm -f $(SOURCES_DIR)/$(SOURCE_ARCHIVE) || true
	rm -f $(SPECS_DIR)/$(PACKAGE_NAME).spec || true

# Clean everything including built packages
distclean: clean
	@echo "Cleaning all built packages..."
	rm -f $(RPMS_DIR)/*/$(PACKAGE_NAME)-*.rpm || true
	rm -f $(SRPMS_DIR)/$(PACKAGE_NAME)-*.src.rpm || true

help:
	@echo "Available targets:"
	@echo "  all           - Build RPM package (default)"
	@echo "  install-deps  - Install RPM build dependencies"
	@echo "  setup-dirs    - Setup RPM build directory structure"
	@echo "  prep          - Prepare source archive"
	@echo "  build-rpm     - Build both binary and source RPM"
	@echo "  build-binary-rpm - Build only binary RPM"
	@echo "  build-source-rpm - Build only source RPM"
	@echo "  check         - Check RPM with rpmlint"
	@echo "  install       - Install the built RPM"
	@echo "  clean         - Clean build artifacts"
	@echo "  distclean     - Clean all built packages"
	@echo "  help          - Show this help message"
