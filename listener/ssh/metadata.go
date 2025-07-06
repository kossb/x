package ssh

import (
	"os"

	mdata "github.com/go-gost/core/metadata"
	ssh_util "github.com/go-gost/x/internal/util/ssh"
	mdutil "github.com/go-gost/x/metadata/util"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/crypto/ssh"
)

const (
	defaultBacklog = 1024  // Increased for high load scenarios
)

type metadata struct {
	signer         ssh.Signer
	authorizedKeys map[string]bool
	backlog        int
	mptcp          bool
}

func (l *sshListener) parseMetadata(md mdata.Metadata) (err error) {
	const (
		authorizedKeys = "authorizedKeys"
		privateKeyFile = "privateKeyFile"
		passphrase     = "passphrase"
		backlog        = "backlog"
	)

	if key := mdutil.GetString(md, privateKeyFile); key != "" {
		key, err = homedir.Expand(key)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(key)
		if err != nil {
			return err
		}

		pp := mdutil.GetString(md, passphrase)
		if pp == "" {
			l.md.signer, err = ssh.ParsePrivateKey(data)
		} else {
			l.md.signer, err = ssh.ParsePrivateKeyWithPassphrase(data, []byte(pp))
		}
		if err != nil {
			return err
		}
	}
	if l.md.signer == nil {
		signer, err := ssh.NewSignerFromKey(l.options.TLSConfig.Certificates[0].PrivateKey)
		if err != nil {
			return err
		}
		l.md.signer = signer
	}

	if name := mdutil.GetString(md, authorizedKeys); name != "" {
		m, err := ssh_util.ParseAuthorizedKeysFile(name)
		if err != nil {
			return err
		}
		l.md.authorizedKeys = m
	}

	l.md.backlog = mdutil.GetInt(md, backlog)
	if l.md.backlog <= 0 {
		l.md.backlog = defaultBacklog
	}

	l.md.mptcp = mdutil.GetBool(md, "mptcp")

	return
}
