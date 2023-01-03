package piano

type Options struct {
	codec                 baseCodec
}

type Option func(*Options)