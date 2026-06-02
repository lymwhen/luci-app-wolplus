#!/usr/bin/env python3
"""Build luci-app-wolplus ipk package without OpenWrt buildroot.

Output: luci-app-wolplus_2.0-<date>_all.ipk
Requires: Python 3.5+ (tarfile, gzip, os, struct — all stdlib)
"""

import os, sys, struct, tarfile, io, hashlib
from datetime import datetime

PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BUILD_DIR = os.path.join(PROJECT_ROOT, "build")
PKG_NAME = "luci-app-wolplus"
PKG_VER = "2.0"
PKG_REL = datetime.now().strftime("%Y%m%d")
PKG_ARCH = "all"

# --- files to package: (source_relpath, dest_abspath, perms) ---
FILES = [
    # LuCI controller / model / view
    ("luasrc/controller/wolplus.lua", "usr/lib/lua/luci/controller/wolplus.lua", 0o644),
    ("luasrc/model/cbi/wolplus.lua",    "usr/lib/lua/luci/model/cbi/wolplus.lua",    0o644),
    ("luasrc/view/wolplus/index.htm",   "usr/lib/lua/luci/view/wolplus/index.htm",   0o644),
    ("luasrc/view/wolplus/awake.htm",   "usr/lib/lua/luci/view/wolplus/awake.htm",   0o644),
    # UCI config
    ("root/etc/config/wolplus",         "etc/config/wolplus",                         0o644),
    # uci-defaults (first-boot setup)
    ("root/etc/uci-defaults/luci-app-wolplus", "etc/uci-defaults/luci-app-wolplus",   0o755),
    # rpcd ACL
    ("root/usr/share/rpcd/acl.d/luci-app-wolplus.json", "usr/share/rpcd/acl.d/luci-app-wolplus.json", 0o644),
    # CGI API (must be executable)
    ("root/www/cgi-bin/wolplus-api",    "www/cgi-bin/wolplus-api",                    0o755),
]


def write_ar(f, name, data):
    """Write an ar archive member with proper header formatting."""
    ts = int(datetime.now().timestamp())
    name_bytes = name.encode("ascii")

    if len(name_bytes) > 15:
        # BSD extended naming: "#1/" + len(name + "/")
        padded = name_bytes + b"/"
        name_field = f"#1/{len(padded)}".ljust(16).encode("ascii")
        payload = padded + data
    else:
        # Short name with System-V trailing '/'
        name_field = (name + "/").ljust(16).encode("ascii")
        payload = data

    size = len(payload)

    # Build 60-byte ar header with proper space-padding for numeric fields
    header = (
        name_field                           # 16 bytes, left-justified
        + f"{ts:12d}".encode()               # 12 bytes, right-justified
        + f"{0:6d}".encode()                 # 6 bytes uid
        + f"{0:6d}".encode()                 # 6 bytes gid
        + f"{100644:8d}".encode()            # 8 bytes mode
        + f"{size:10d}".encode()             # 10 bytes size
        + b"\x60\x0A"                        # 2 bytes magic
    )

    assert len(header) == 60, f"ar header length {len(header)} != 60"

    f.write(header)
    f.write(payload)
    # ar pads data to 2-byte boundary
    if size % 2:
        f.write(b"\n")


def build_control_tar():
    """Build control.tar.gz bytes."""
    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode="w:gz") as tar:

        # control file
        ctrl = (
            f"Package: {PKG_NAME}\n"
            f"Version: {PKG_VER}-{PKG_REL}\n"
            f"Architecture: {PKG_ARCH}\n"
            f"Depends: libc, etherwake, curl\n"
            f"Maintainer: sundaqiang\n"
            f"Description: LuCI support for WolPlus - Wake-on-LAN with status detection and remote shutdown\n"
        )
        ti = tarfile.TarInfo("./control")
        ti.size = len(ctrl)
        ti.mode = 0o644
        tar.addfile(ti, io.BytesIO(ctrl.encode()))

        # postinst script
        postinst = (
            "#!/bin/sh\n"
            '[ -n "${IPKG_INSTROOT}" ] && exit 0\n'
            "chmod 755 /www/cgi-bin/wolplus-api 2>/dev/null\n"
            "if ! uci -q get uhttpd.main.interpreter | grep -q '\\.lua=/usr/bin/lua'; then\n"
            "\tuci -q add_list uhttpd.main.interpreter='.lua=/usr/bin/lua'\n"
            "\tuci commit uhttpd\n"
            "\t/etc/init.d/uhttpd restart 2>/dev/null\n"
            "fi\n"
            "exit 0\n"
        )
        ti = tarfile.TarInfo("./postinst")
        ti.size = len(postinst)
        ti.mode = 0o755
        tar.addfile(ti, io.BytesIO(postinst.encode()))

        # conffiles (marks config files so they survive upgrades)
        conffiles = "/etc/config/wolplus\n"
        ti = tarfile.TarInfo("./conffiles")
        ti.size = len(conffiles)
        ti.mode = 0o644
        tar.addfile(ti, io.BytesIO(conffiles.encode()))

    return buf.getvalue()


def build_data_tar():
    """Build data.tar.gz bytes with all package files."""
    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode="w:gz") as tar:
        for src_rel, dst_path, perms in FILES:
            src = os.path.join(PROJECT_ROOT, src_rel)
            if not os.path.exists(src):
                print(f"  WARN: source not found: {src_rel}")
                continue

            with open(src, "rb") as fh:
                content = fh.read()

            # Ensure POSIX path separators
            arcname = "./" + dst_path.replace("\\", "/")

            ti = tarfile.TarInfo(arcname)
            ti.size = len(content)
            ti.mode = perms
            ti.uid = 0
            ti.gid = 0
            ti.uname = "root"
            ti.gname = "root"
            tar.addfile(ti, io.BytesIO(content))
            print(f"  + {dst_path}")

    return buf.getvalue()


def main():
    os.makedirs(BUILD_DIR, exist_ok=True)

    debian_binary = b"2.0\n"
    control_tar = build_control_tar()
    data_tar = build_data_tar()

    ipk_name = f"{PKG_NAME}_{PKG_VER}-{PKG_REL}_{PKG_ARCH}.ipk"
    ipk_path = os.path.join(BUILD_DIR, ipk_name)

    with open(ipk_path, "wb") as f:
        f.write(b"!<arch>\n")
        write_ar(f, "debian-binary", debian_binary)
        write_ar(f, "control.tar.gz", control_tar)
        write_ar(f, "data.tar.gz", data_tar)

    size_kb = os.path.getsize(ipk_path) / 1024
    print(f"\n  -> {ipk_path}  ({size_kb:.1f} KB)")

    # Also create a raw tar.gz for direct extraction (useful for manual install)
    tgz_name = f"{PKG_NAME}_{PKG_VER}-{PKG_REL}_raw.tar.gz"
    tgz_path = os.path.join(BUILD_DIR, tgz_name)
    with open(tgz_path, "wb") as f:
        f.write(data_tar)
    print(f"  -> {tgz_path}  (raw data.tar.gz, extract to /)")


if __name__ == "__main__":
    main()
