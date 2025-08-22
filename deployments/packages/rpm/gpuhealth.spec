Name:           gpuhealth
Version:        0.1.0
Release:        1%{?dist}
Summary:        GPU Health monitoring agent for datacenter environments

License:        Apache-2.0
URL:            https://github.com/leptonai/gpud
Source0:        %{name}-%{version}.tar.gz
BuildRequires:  make, golang

Requires:       systemd >= 230
Requires:       curl
Suggests:       nvidia-driver >= 535

%description
GPU Health Agent provides comprehensive monitoring for NVIDIA datacenter GPUs
including Hopper and Blackwell generations.

Users can optionally connect to NVIDIA GPU Global health platform for
enhanced GPU health insights, predictive analysis, and centralized monitoring
across multiple systems.

%prep
%setup -q

%build
# Build the gpuhealth binary
make bin/gpuhealth

%install
# Create directories
install -d %{buildroot}%{_bindir}
install -d %{buildroot}%{_unitdir}
install -d %{buildroot}%{_sysconfdir}/default
install -d %{buildroot}%{_sharedstatedir}/gpuhealth

# Install binary
install -m 0755 bin/gpuhealth %{buildroot}%{_bindir}/gpuhealth

# Install systemd service file
install -m 0644 deployments/packages/common/systemd/gpuhealthd.service %{buildroot}%{_unitdir}/gpuhealthd.service

# Install environment configuration
install -m 0644 deployments/packages/common/systemd/gpuhealth.env %{buildroot}%{_sysconfdir}/default/gpuhealth

# No pre-installation script needed - service runs as root

%pre
# Pre-installation script (warn if systemd not available)
if [ ! -d /run/systemd/system ]; then
    echo "[WARNING] systemd not detected - service management will be limited"
fi

%post
# Post-installation script
if [ -d /run/systemd/system ]; then
    systemctl daemon-reload >/dev/null 2>&1 || :
    if [ $1 -eq 1 ] ; then
        # Initial installation
        systemctl enable gpuhealthd.service >/dev/null 2>&1 || :
        systemctl start gpuhealthd.service >/dev/null 2>&1 || :
    else
        # Upgrade
        if systemctl is-active --quiet gpuhealthd.service; then
            systemctl restart gpuhealthd.service >/dev/null 2>&1 || :
        else
            systemctl start gpuhealthd.service >/dev/null 2>&1 || :
        fi
    fi
fi

# Setup directories and permissions
mkdir -p %{_sharedstatedir}/gpuhealth
chmod 755 %{_sharedstatedir}/gpuhealth %{_bindir}/gpuhealth

echo "GPU Health Agent installed successfully!"
echo ""
echo "Configuration: sudo vi %{_sysconfdir}/default/gpuhealth"
echo "Start service: sudo systemctl start gpuhealthd"
echo "Enable service: sudo systemctl enable gpuhealthd"
echo "Check logs: sudo journalctl -u gpuhealthd -f"
echo "Documentation: rpm -qd %{name}"
echo ""

%preun
# Pre-uninstallation script
if [ $1 -eq 0 ] ; then
    # Package removal, not upgrade
    systemctl --no-reload disable gpuhealthd.service > /dev/null 2>&1 || :
    systemctl stop gpuhealthd.service > /dev/null 2>&1 || :
fi

%postun
# Post-uninstallation script
systemctl daemon-reload >/dev/null 2>&1 || :
if [ $1 -ge 1 ] ; then
    # Package upgrade, not removal
    systemctl try-restart gpuhealthd.service >/dev/null 2>&1 || :
fi

if [ $1 -eq 0 ] ; then
    # Complete removal - clean up data/config directory
    rm -rf %{_sharedstatedir}/gpuhealth
    rm -f %{_sysconfdir}/default/gpuhealth
fi

%files
%license LICENSE
%{_bindir}/gpuhealth
%{_unitdir}/gpuhealthd.service
%config(noreplace) %{_sysconfdir}/default/gpuhealth
%dir %{_sharedstatedir}/gpuhealth

%changelog
* Mon Jan 01 2024 GPU Health Team <gpuhealth@exchange.nvidia.com> - 0.1.0
- Initial RPM package release
- GPU Health monitoring agent for datacenter environments
- Support for NVIDIA Hopper and Blackwell generations
- Systemd service integration
- Configuration management via /etc/default/gpuhealth
