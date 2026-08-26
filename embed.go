package main

import (
	"cmp"
	_ "embed"
	"errors"
	"fmt"
	"math"
	"os"
	"slices"
	"sync"
	"unicode/utf8"

	"github.com/gomlx/go-huggingface/tokenizers/api"
	"github.com/gomlx/go-huggingface/tokenizers/hftokenizer"
	ort "github.com/yalue/onnxruntime_go"
)

// The transformer and its tokenizer travel inside the binary. A runtime
// download would put a second artifact between installing slopguard and having
// it work, and a hook that pauses to fetch 86 MB on its first call is worse
// than a hook that is simply larger.

//go:embed assets/model.onnx
var modelBytes []byte

//go:embed assets/tokenizer.json
var tokenizerBytes []byte

const (
	// dimensions is the width of an all-MiniLM-L6-v2 vector.
	dimensions = 384
	// budget is the token cap the model was fine-tuned and published at.
	// A comment longer than this is truncated rather than chunked: the opening
	// sentences are what the rules read.
	budgetTokens = 256
	// libraryPathEnv names the shared library outright, for a machine that
	// keeps it somewhere none of [libraryPaths] looks.
	libraryPathEnv = "SLOPGUARD_ONNXRUNTIME_LIBRARY"
	// disableEnv turns the semantic pass off, leaving the syntactic rules and
	// the phrase lists.
	disableEnv = "SLOPGUARD_NO_MODEL"
)

// The graph's interface, stated rather than read off the model: asking ONNX
// Runtime for it costs a whole session that is then discarded.
var (
	inputNames  = []string{"input_ids", "attention_mask", "token_type_ids"}
	outputNames = []string{"last_hidden_state"}
)

// An embedder turns text into unit-length vectors, so that a dot product is a
// cosine everywhere downstream. It is loaded once per process and only when a
// comment actually needs judging.
type embedder struct {
	session *ort.DynamicAdvancedSession
	options *ort.SessionOptions
	tk      api.Tokenizer
	padID   int64
}

var (
	loadOnce  sync.Once
	loaded    *embedder
	loadedErr error
)

// model returns the process's embedder, loading ONNX Runtime and the graph on
// the first call. Measured warm, that is about 90 ms of an invocation that
// reaches it; a hook that finds no comment to judge never pays it and returns
// in about 5 ms.
func model() (*embedder, error) {
	// Read outside the Once: a process that consults the variable only on its
	// first call would answer later ones from whatever it happened to hold
	// then, which in a test run is whichever test got there first.
	if os.Getenv(disableEnv) != "" {
		return nil, errors.New("the semantic pass is off")
	}
	loadOnce.Do(func() {
		loaded, loadedErr = load()
	})
	return loaded, loadedErr
}

// libraryPaths are where ONNX Runtime is looked for, in order: Homebrew on
// Apple Silicon, Homebrew on Intel, Linuxbrew, and the two places a Linux
// package manager puts it. Hardcoding the first of them meant the formula could
// depend on onnxruntime and the binary still fail to find it on every machine
// but one.
var libraryPaths = []string{
	"/opt/homebrew/lib/libonnxruntime.dylib",
	"/usr/local/lib/libonnxruntime.dylib",
	"/home/linuxbrew/.linuxbrew/lib/libonnxruntime.so",
	"/usr/lib/x86_64-linux-gnu/libonnxruntime.so",
	"/usr/local/lib/libonnxruntime.so",
}

// library returns the first shared library that is there, or the last place
// looked so that the failure names something.
func library() string {
	if override := os.Getenv(libraryPathEnv); override != "" {
		return override
	}
	for _, path := range libraryPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return libraryPaths[len(libraryPaths)-1]
}

func load() (*embedder, error) {
	path := library()
	ort.SetSharedLibraryPath(path)
	if err := ort.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("open ONNX Runtime at %s: %w", path, err)
	}
	tk, err := hftokenizer.NewFromContent(nil, tokenizerBytes)
	if err != nil {
		return nil, fmt.Errorf("read the embedded tokenizer: %w", err)
	}
	if err := tk.With(api.EncodeOptions{AddSpecialTokens: true}); err != nil {
		return nil, fmt.Errorf("configure the embedded tokenizer: %w", err)
	}
	padID, err := tk.SpecialTokenID(api.TokPad)
	if err != nil {
		padID = 0
	}
	options, err := ort.NewSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("build the session options: %w", err)
	}
	session, err := ort.NewDynamicAdvancedSessionWithONNXData(modelBytes, inputNames, outputNames, options)
	if err != nil {
		_ = options.Destroy()
		return nil, fmt.Errorf("build the session over the embedded model: %w", err)
	}
	return &embedder{session: session, options: options, tk: tk, padID: int64(padID)}, nil
}

// chunk is how many texts go through the transformer at once. A batch is padded
// to its own longest member and the output tensor is one allocation of
// batch × width × 384 floats, so an unchunked batch of a whole file's comments
// would ask ONNX Runtime for gigabytes.
const chunk = 32

// embed returns a unit-length vector for each text, mask-weighted mean pooled
// over the token positions, running them through the transformer a chunk at a
// time.
//
// Texts of a like length travel together. A batch is padded to its longest
// member and inference is linear in the padded width, so one long sentence
// among thirty short ones would otherwise cost thirty long ones.
func (e *embedder) embed(texts []string) ([][]float32, error) {
	order := make([]int, len(texts))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		return cmp.Compare(len(texts[a]), len(texts[b]))
	})
	out := make([][]float32, len(texts))
	for start := 0; start < len(order); start += chunk {
		end := min(start+chunk, len(order))
		batch := make([]string, 0, end-start)
		for _, i := range order[start:end] {
			batch = append(batch, texts[i])
		}
		vectors, err := e.forward(batch)
		if err != nil {
			return nil, err
		}
		for k, i := range order[start:end] {
			out[i] = vectors[k]
		}
	}
	return out, nil
}

// budgetBytes is how much text is handed to the tokenizer.
//
// Truncating the token ids afterwards bounds what the model sees and not what
// the tokenizer does, and the tokenizer is quadratic in its input: a comment
// with no sentence punctuation in it — a base64 blob, a generated table —
// arrives as one unit, and 900 KB of it ran for nine minutes before it was
// killed. No sentence worth reading survives past this many bytes anyway.
//
// It is set from what the tokenizer yields rather than from four bytes to a
// token: at 1536 bytes the thinnest input measured still gives 214 ids against
// a budget of 256, and the densest gives 834, while the worst case costs a
// twentieth of what 4096 did. The quadratic is still there, bounded lower.
const budgetBytes = 1536

// clip cuts text to [budgetBytes] on a rune boundary.
func clip(text string) string {
	if len(text) <= budgetBytes {
		return text
	}
	cut := budgetBytes
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut]
}

// forward runs one batch, padded to its own longest member.
func (e *embedder) forward(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	rows := make([][]int64, len(texts))
	width := 0
	for i, text := range texts {
		ids := e.tk.Encode(clip(text))
		if len(ids) > budgetTokens {
			ids = ids[:budgetTokens]
		}
		rows[i] = make([]int64, len(ids))
		for j, id := range ids {
			rows[i][j] = int64(id)
		}
		width = max(width, len(ids))
	}
	if width == 0 {
		return nil, fmt.Errorf("nothing to embed")
	}

	shape := ort.NewShape(int64(len(texts)), int64(width))
	inputs := make([]ort.Value, len(inputNames))
	defer destroy(inputs)
	for i, name := range inputNames {
		backing := make([]int64, len(texts)*width)
		for b, ids := range rows {
			row := backing[b*width : (b+1)*width]
			switch name {
			case "input_ids":
				copy(row, ids)
				for s := len(ids); s < width; s++ {
					row[s] = e.padID
				}
			case "attention_mask":
				for s := range ids {
					row[s] = 1
				}
			case "token_type_ids":
			default:
				return nil, fmt.Errorf("the embedded model wants an input slopguard does not supply: %q", name)
			}
		}
		tensor, err := ort.NewTensor(shape, backing)
		if err != nil {
			return nil, fmt.Errorf("build the %s tensor: %w", name, err)
		}
		inputs[i] = tensor
	}

	outputs := make([]ort.Value, len(outputNames))
	defer destroy(outputs)
	if err := e.session.Run(inputs, outputs); err != nil {
		return nil, fmt.Errorf("run the model: %w", err)
	}

	hidden, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, fmt.Errorf("the model returned %T rather than a float tensor", outputs[0])
	}
	flat := hidden.GetData()
	if want := len(texts) * width * dimensions; len(flat) != want {
		return nil, fmt.Errorf("the model returned %d values, expected %d", len(flat), want)
	}
	vectors := make([][]float32, len(texts))
	for b := range texts {
		vectors[b] = pool(flat[b*width*dimensions:(b+1)*width*dimensions], len(rows[b]))
	}
	return vectors, nil
}

// pool averages the hidden states of the content positions and scales the
// result to unit length. Padding positions carry real activations, so folding
// them in would inflate every similarity.
func pool(states []float32, content int) []float32 {
	vec := make([]float32, dimensions)
	for s := range content {
		row := states[s*dimensions : (s+1)*dimensions]
		for k, v := range row {
			vec[k] += v
		}
	}
	if content > 0 {
		for k := range vec {
			vec[k] /= float32(content)
		}
	}
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	if norm = math.Sqrt(norm); norm > 0 {
		for k := range vec {
			vec[k] = float32(float64(vec[k]) / norm)
		}
	}
	return vec
}

func destroy(values []ort.Value) {
	for _, v := range values {
		if v != nil {
			_ = v.Destroy()
		}
	}
}
