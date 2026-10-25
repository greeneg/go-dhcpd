%global go_version 1.27rc3

Name:           go-dhcpd
Version:        1.9.1
Release:        1%{?dist}
Summary:        A modern DHCP and BootP daemon written in Go

License:        Apache-2.0
URL:            https://github.com/greeneg/go-dhcpd
# GitHub strips the leading "v" from version-like tags when naming the
# archive's top-level directory, so this unpacks as %%{name}-%%{version}/.
Source0:        https://github.com/greeneg/go-dhcpd/archive/refs/tags/v%{version}.tar.gz#/%{name}-%{version}.tar.gz

# NOTE: go.mod declares "go %%{go_version}" as its minimum toolchain. With
# GOTOOLCHAIN=auto (the Go default), `go build` fetches a matching toolchain
# automatically if the one below is older -- but that requires network
# access during %%build. Offline/mock builds must provide a golang package
# that already satisfies this directive.
BuildRequires:  golang >= 1.21
BuildRequires:  gcc
BuildRequires:  pam-devel
BuildRequires:  systemd-rpm-macros
Requires:       pam
# internal/allocator shells out to these for dynamic-address conflict
# detection (ping) and MAC-of-squatter reporting (arp); a missing ping is
# silently treated as "address free", so these must not be optional.
Requires:       /usr/bin/ping
Requires:       /usr/sbin/arp
%{?systemd_requires}

# mattn/go-sqlite3 compiles its own bundled sqlite3 amalgamation via cgo, so
# no sqlite-devel BuildRequires is needed; msteinert/pam/v2 is what requires
# pam-devel/gcc above.

%description
go-dhcpd is a DHCP and BootP server implementing RFC 2131, RFC 2132, RFC 951,
and related option-format RFCs. It allocates dynamic and static IPv4 leases,
tracks them in a local SQLite3 database, and exposes a REST API (with PAM
Basic Auth and optional TLS) for monitoring and administration.

Subnets and static leases are supplied by an external, independently
executed config-provider plugin; this package bundles a read-only
JSON5-file-backed plugin (file-config) and a read/write SQLite3-backed
plugin (sqlite3-config), plus an offline migration tool to move data
between them.

%prep
%autosetup -n %{name}-%{version}

%build
# Keep Go's cache/module directories inside the build tree: the default
# $HOME-relative locations are not guaranteed writable in restricted build
# environments (e.g. mock/koji).
export GOCACHE=%{_builddir}/%{name}-%{version}/.cache/go-build
export GOPATH=%{_builddir}/%{name}-%{version}/.gopath
make build INSTALL_PREFIX=%{_prefix}

%install
export GOCACHE=%{_builddir}/%{name}-%{version}/.cache/go-build
export GOPATH=%{_builddir}/%{name}-%{version}/.gopath
make install DESTDIR=%{buildroot} INSTALL_PREFIX=%{_prefix}

install -D -m 0644 go-dhcpd.service %{buildroot}%{_unitdir}/go-dhcpd.service
# The shipped unit assumes the manual /usr/local install path; retarget it
# to this package's actual binary location.
sed -i 's#/usr/local/bin/dhcpd#%{_bindir}/dhcpd#' %{buildroot}%{_unitdir}/go-dhcpd.service

# `make install`'s own PAM-stack detection only installs
# %{_sysconfdir}/pam.d/go-dhcpd if the *build host* already has
# system-auth or common-auth; a minimal buildroot (e.g. mock) can have
# neither, which would silently skip it and break the unconditional
# %%files entry below. Install the target distribution's profile
# explicitly instead, so it's always present regardless of build host.
%if 0%{?suse_version}
install -D -m 0644 pam.d/go-dhcpd.common-auth %{buildroot}%{_sysconfdir}/pam.d/go-dhcpd
%else
install -D -m 0644 pam.d/go-dhcpd.system-auth %{buildroot}%{_sysconfdir}/pam.d/go-dhcpd
%endif

%check
export GOCACHE=%{_builddir}/%{name}-%{version}/.cache/go-build
export GOPATH=%{_builddir}/%{name}-%{version}/.gopath
make lint

%post
%systemd_post go-dhcpd.service

%preun
%systemd_preun go-dhcpd.service

%postun
%systemd_postun_with_restart go-dhcpd.service

%files
%license LICENSE
%doc README.md Docs/
%{_bindir}/dhcpd
%{_bindir}/go-dhcpd-migration-tool
%dir %{_sysconfdir}/go-dhcpd
%config(noreplace) %{_sysconfdir}/go-dhcpd/config.json5.example
%dir %{_localstatedir}/lib/go-dhcpd
# NOTE: the Makefile installs plugins under "<prefix>/lib", not the
# multilib-aware %%{_libdir} (e.g. /usr/lib64 on x86_64); this matches
# what `make install` actually produces.
%dir %{_prefix}/lib/go-dhcpd
%dir %{_prefix}/lib/go-dhcpd/plugins
%{_prefix}/lib/go-dhcpd/plugins/file-config.plugin
%{_prefix}/lib/go-dhcpd/plugins/sqlite3-config.plugin
%{_mandir}/man8/dhcpd.8*
%{_mandir}/man8/go-dhcpd-migration-tool.8*
%{_mandir}/man5/go-dhcpd-config.json5.5*
%{_mandir}/man3/go-dhcpd-file-config.3*
%{_mandir}/man3/go-dhcpd-sqlite3-config.3*
%config(noreplace) %{_sysconfdir}/pam.d/go-dhcpd
%{_unitdir}/go-dhcpd.service

%changelog
* Sun Oct 25 2026 Gary L. Greene Jr. <greeneg@yggdrasilsoft.com> - 1.9.1-1
- Add bootp packet statistics

* Sun Oct 25 2026 Gary L. Greene Jr. <greeneg@yggdrasilsoft.com> - 1.9.0-1
- Initial RPM packaging
