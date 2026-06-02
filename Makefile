# Copyright (C) 2016 Openwrt.org
#
# This is free software, licensed under the Apache License, Version 2.0 .
#

include $(TOPDIR)/rules.mk

LUCI_TITLE:=LuCI support for wolplus From sundaqiang
LUCI_DEPENDS:=+etherwake +curl
LUCI_PKGARCH:=all
PKG_VERSION:=2.0
PKG_RELEASE:=20260520
PKG_MAINTAINER:=sundaqiang

include $(TOPDIR)/feeds/luci/luci.mk

define Package/luci-app-wolplus/postinst
#!/bin/sh
[ "$${IPKG_NO_SCRIPT}" = "1" ] && exit 0
[ -n "$${IPKG_INSTROOT}" ] && exit 0
chmod 755 /www/cgi-bin/wolplus-api 2>/dev/null
if ! uci -q get uhttpd.main.interpreter | grep -q '\.lua=/usr/bin/lua'; then
	uci -q add_list uhttpd.main.interpreter='.lua=/usr/bin/lua'
	uci commit uhttpd
fi
rm -f /tmp/luci-indexcache
/etc/init.d/uhttpd restart 2>/dev/null
exit 0
endef

# call BuildPackage - OpenWrt buildroot signature
