package noise

import (
	"crypto/rand"
	"sync"
	"sync/atomic"

	"github.com/flynn/noise"

	"github.com/skycoin/skywire/pkg/cipher"
)

// packetsTillRekey is the number of packages after which we want to rekey for the noise protocol
const packetsTillRekey = 10

// Config hold noise parameters.
type Config struct {
	LocalPK   cipher.PubKey // Local instance static public key.
	LocalSK   cipher.SecKey // Local instance static secret key.
	RemotePK  cipher.PubKey // Remote instance static public key.
	Initiator bool          // Whether the local instance initiates the connection.
}

// Noise handles the handshake and the frame's cryptography.
type Noise struct {
	pk   cipher.PubKey
	sk   cipher.SecKey
	init bool

	pattern noise.HandshakePattern
	hs      *noise.HandshakeState
	enc     *noise.CipherState
	dec     *noise.CipherState

	encN uint32
	decN uint32

	encMu sync.Mutex
	decMu sync.Mutex
}

// New creates a new Noise with:
//	- provided pattern for handshake.
//	- Secp256k1 for the curve.
func New(pattern noise.HandshakePattern, config Config) (*Noise, error) {
	nc := noise.Config{
		CipherSuite: noise.NewCipherSuite(Secp256k1{}, noise.CipherChaChaPoly, noise.HashSHA256),
		Random:      rand.Reader,
		Pattern:     pattern,
		Initiator:   config.Initiator,
		StaticKeypair: noise.DHKey{
			Public:  config.LocalPK[:],
			Private: config.LocalSK[:],
		},
	}
	if !config.RemotePK.Null() {
		nc.PeerStatic = config.RemotePK[:]
	}

	hs, err := noise.NewHandshakeState(nc)
	if err != nil {
		return nil, err
	}
	return &Noise{
		pk:      config.LocalPK,
		sk:      config.LocalSK,
		init:    config.Initiator,
		pattern: pattern,
		hs:      hs,
	}, nil
}

// KKAndSecp256k1 creates a new Noise with:
//	- KK pattern for handshake.
//	- Secp256k1 for the curve.
func KKAndSecp256k1(config Config) (*Noise, error) {
	return New(noise.HandshakeKK, config)
}

// XKAndSecp256k1 creates a new Noise with:
//  - XK pattern for handshake.
//	- Secp256 for the curve.
func XKAndSecp256k1(config Config) (*Noise, error) {
	return New(noise.HandshakeXK, config)
}

// HandshakeMessage generates handshake message for a current handshake state.
func (ns *Noise) HandshakeMessage() (res []byte, err error) {
	if ns.hs.MessageIndex() < len(ns.pattern.Messages)-1 {
		res, _, _, err = ns.hs.WriteMessage(nil, nil)
		return
	}

	res, ns.dec, ns.enc, err = ns.hs.WriteMessage(nil, nil)
	return res, err
}

// ProcessMessage processes a received handshake message and appends the payload.
func (ns *Noise) ProcessMessage(msg []byte) (err error) {
	if ns.hs.MessageIndex() < len(ns.pattern.Messages)-1 {
		_, _, _, err = ns.hs.ReadMessage(nil, msg)
		return
	}

	_, ns.enc, ns.dec, err = ns.hs.ReadMessage(nil, msg)
	return err
}

// LocalStatic returns the local static public key.
func (ns *Noise) LocalStatic() cipher.PubKey {
	return ns.pk
}

// RemoteStatic returns the remote static public key.
func (ns *Noise) RemoteStatic() cipher.PubKey {
	pk, err := cipher.NewPubKey(ns.hs.PeerStatic())
	if err != nil {
		panic(err)
	}
	return cipher.PubKey(pk)
}

// EncryptUnsafe encrypts plaintext without interlocking, should only
// be used with external lock.
func (ns *Noise) EncryptUnsafe(plaintext []byte) []byte {
	atomic.AddUint32(&ns.encN, 1)
	if atomic.CompareAndSwapUint32(&ns.encN, packetsTillRekey, 0) {
		ns.enc.Rekey()
	}

	return ns.enc.Encrypt(nil, nil, plaintext)
}

// Encrypt encrypts plaintext.
func (ns *Noise) Encrypt(plaintext []byte) []byte {
	ns.encMu.Lock()
	res := ns.EncryptUnsafe(plaintext)
	ns.encMu.Unlock()
	return res
}

// DecryptUnsafe decrypts ciphertext without interlocking, should only
// be used with external lock.
func (ns *Noise) DecryptUnsafe(ciphertext []byte) ([]byte, error) {
	atomic.AddUint32(&ns.decN, 1)
	if atomic.CompareAndSwapUint32(&ns.decN, packetsTillRekey, 0) {
		ns.dec.Rekey()
	}

	return ns.dec.Decrypt(nil, nil, ciphertext)
}

// Decrypt decrypts ciphertext.
func (ns *Noise) Decrypt(ciphertext []byte) ([]byte, error) {
	ns.decMu.Lock()
	res, err := ns.DecryptUnsafe(ciphertext)
	ns.decMu.Unlock()
	return res, err
}

// HandshakeFinished indicate whether handshake was completed.
func (ns *Noise) HandshakeFinished() bool {
	return ns.hs.MessageIndex() == len(ns.pattern.Messages)
}