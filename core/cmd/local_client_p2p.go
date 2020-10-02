package cmd

import (
	"fmt"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/pkg/errors"
	clipkg "github.com/urfave/cli"

	"github.com/smartcontractkit/chainlink/core/store/orm"
)

// CreateP2PKey creates a key and inserts it into encrypted_p2p_keys,
// protected by the password in the password file
func (cli *Client) CreateP2PKey(c *clipkg.Context) error {
	return cli.errorOut(cli.createP2PKey(c))
}

const createKeyMsg = `Created P2P keypair.
Key ID
  %v
Public key:
  0x%x
Peer ID:
  %s
`

func (cli *Client) createP2PKey(c *clipkg.Context) error {
	cli.Config.Dialect = orm.DialectPostgresWithoutLock
	store := cli.AppFactory.NewApplication(cli.Config).GetStore()

	password, err := getPassword(c)
	if err != nil {
		return err
	}
	_, enc, err := store.OCRKeyStore.GenerateEncryptedP2PKey(string(password))
	if err != nil {
		return errors.Wrap(err, "while generating encrypted p2p key")
	}
	fmt.Printf(createKeyMsg, enc.ID, enc.PubKey, peer.ID(enc.PeerID).Pretty())
	return nil
}
