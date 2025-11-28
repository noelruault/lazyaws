# lazyaws

A terminal UI for managing AWS resources (EC2, S3, EKS, ECS, ECR, Secrets Manager).

## Usage

Press `:help` or `?` for keyboard shortcuts. Use `:services` (or `tab`/`esc` from a service screen) to jump back to the service picker.

### Quick Start

**Connect to EC2 via SSM:**

1. Press `/` to search for instance
2. Press `c` to connect
3. Ctrl+C works inside session (kills commands, not the session)
4. `exit` to return to lazyaws

**Edit S3 file:**

1. Navigate to object
2. Press `e` to edit in $EDITOR
3. Save and quit
4. File automatically uploads
5. Returns to same S3 location

**Key Shortcuts:**

```
j/k, ↑/↓      Navigate
g/G           Top/bottom
Ctrl+d/u      Page down/up
/             Search
n/N           Next/prev match
Enter         Select/view details
Esc/q         Back/quit
:help         Show help
```

**EC2:**

```
s/S           Start/stop
r/t           Reboot/terminate
c             SSM connect
9             Launch k9s (EKS nodes)
Space         Multi-select
```

**S3:**

```
e             Edit file in $EDITOR
d             Download
D             Delete (typed confirmation)
u             Presigned URL
p/v           Policy/versioning
```

**EKS:**

```
9             Launch k9s
u             Update kubeconfig
```

**ECS:**

```
Enter         Services → Tasks
r             Refresh
```

**ECR:**

```
Enter         Repos → Images → Scan
r             Refresh (list/scan)
```

**Secrets Manager:**

```
Enter         Secret details
r             Refresh (list/details)
```
