# The demonstration computer (Proxmox VM)

The computer behind `desk.go-link.org/demo`: what App Store and Google Play reviewers, and anyone, connect to. It has to be safe to hand to strangers, so it is locked down in layers; one failing leaves the others.

| Layer | What |
|---|---|
| **The desktop** | The user `demo` logs in by itself into Openbox with Minesweeper, a calculator and a text editor. No browser, terminal, file manager, help viewer or `xdg-open`. No password, no sudo; polkit refuses it everything (power, mounts, updates); no switching to a text console (`DontVTSwitch`). |
| **Its files** | `/home/demo` is a tmpfs (`noexec`): whatever someone saves is gone at the next start, and the computer restarts every night (`demo-nightly-reboot.timer`). |
| **demo's network** | nftables drops everything the user `demo` sends but to the computer itself. |
| **The computer's network** | The Proxmox firewall (`pve-firewall-106.fw`) lets the VM reach the signaling server and the relay (54.208.64.177, one machine) and DNS, nothing else: not the local network, not the rest of the internet; SSH in from the local network only, keys only. The host carries every session through the relay (`--test-relay-only`) so this is enough. |
| **go-link Desk** | Runs as `golink` (`go-link-desk-demo.service`, systemd-hardened) and shares demo's screen (`--test-demo-screen`): the viewers' mouse and keyboard reach that desktop only; the clipboard is a file and files stay in `/home/golink/files` (25 MB at most, emptied every few minutes), which demo cannot read. |
| **No credentials** | The VM keeps none. The Proxmox host reads `/var/lib/go-link-demo/demo.json` over SSH with a key whose only command is that `cat`, and publishes it (`pve-publish.*`, `../publish.py`) with the AWS key that may write `desk.go-link.org/demo.json` only (IAM user `go-link-desk-demo`, in `/root/.config/desk-demo/aws.env` on the Proxmox host). |

## Build it

1. On the Proxmox host, with the administrator's key in `cloud-init.yaml` (`SSH_PUBLIC_KEY`):

   ```sh
   cp cloud-init.yaml /var/lib/vz/snippets/desk-demo.yaml
   qm create 106 --name desk-demo --memory 3072 --cores 2 --cpu host --machine q35 \
     --net0 virtio,bridge=vmbr0,firewall=1 --scsihw virtio-scsi-pci --vga virtio \
     --serial0 socket --agent enabled=1 --ostype l26 --onboot 1
   qm importdisk 106 /var/lib/vz/template/iso/noble-server-cloudimg-amd64.img local-lvm
   qm set 106 --scsi0 local-lvm:vm-106-disk-0 --boot order=scsi0 --ide2 local-lvm:cloudinit \
     --ipconfig0 ip=172.17.0.42/24,gw=172.17.0.1 --nameserver 8.8.8.8 \
     --cicustom user=local:snippets/desk-demo.yaml
   qm resize 106 scsi0 16G && qm start 106
   ```

2. Before the firewall is on (it needs the internet for its packages), on the VM as `deskadmin`: the Linux release's binary in `/opt/go-link-desk-demo`, `go-link-desk-demo.service` and the clean timer in `/etc/systemd/system`, `welcome.txt` in `/home/golink/files`, `DEMO_PASSWORD=...` in `/etc/go-link-demo/demo.env`, `/var/lib/go-link-demo` owned by golink; enable the service and the timer.
3. On the Proxmox host: a key for reading the codes (`ssh-keygen -t ed25519 -f /root/.ssh/desk-demo-pull`), added to deskadmin's `authorized_keys` as `command="cat /var/lib/go-link-demo/demo.json",no-pty,no-port-forwarding,no-agent-forwarding,no-X11-forwarding,no-user-rc <key>`; `publish.py` and `pve-publish.sh` in `/usr/local/lib/desk-demo`, `pve-publish.service` and `.timer` as `desk-demo-publish.*` in `/etc/systemd/system`; `aws.env` (mode 600); `systemctl enable --now desk-demo-publish.timer`.
4. The firewall: `pve-firewall-106.fw` as `/etc/pve/firewall/106.fw`, and `/etc/pve/firewall/cluster.fw` with `enable: 1`, `policy_in: ACCEPT`, `policy_out: ACCEPT` (only VMs with `firewall=1` on their network card get rules).

## Updating it

The firewall keeps the VM from Ubuntu's mirrors. To update: comment out `policy_out: DROP` in `106.fw` (or add the mirrors), `sudo apt-get update && sudo apt-get upgrade`, and put it back.
