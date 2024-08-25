package main

type GGUFOptions struct {
	General   GeneralOptions
	Sampling  SamplingOptions
	Grammar   GrammarOptions
	Embedding EmbeddingOptions
	Context   ContextOptions
	Model     ModelOptions
	Retrieval RetrievalOptions
	Passkey   PasskeyOptions
	IMatrix   IMatrixOptions
	Bench     BenchOptions
	Server    ServerOptions
	Logging   LoggingOptions
	CVector   CVectorOptions
}

type GeneralOptions struct {
	Help               bool     `flag:"help,h"`
	Version            bool     `flag:"version"`
	Verbose            bool     `flag:"verbose,v"`
	Verbosity          *int     `flag:"verbosity"`
	VerbosePrompt      bool     `flag:"verbose-prompt"`
	NoDisplayPrompt    bool     `flag:"no-display-prompt"`
	Color              bool     `flag:"color,co"`
	Seed               *int     `flag:"seed,s"`
	Threads            *int     `flag:"threads,t"`
	ThreadsBatch       *int     `flag:"threads-batch,tb"`
	ThreadsDraft       *int     `flag:"threads-draft,td"`
	ThreadsBatchDraft  *int     `flag:"threads-batch-draft,tbd"`
	Draft              *int     `flag:"draft"`
	PSplit             *float64 `flag:"p-split,ps"`
	LookupCacheStatic  *string  `flag:"lookup-cache-static,lcs"`
	LookupCacheDynamic *string  `flag:"lookup-cache-dynamic,lcd"`
	CtxSize            *int     `flag:"ctx-size,c"`
	Predict            *int     `flag:"predict,n"`
	BatchSize          *int     `flag:"batch-size,b"`
	UBatchSize         *int     `flag:"ubatch-size,ub"`
	Keep               *int     `flag:"keep"`
	Chunks             *int     `flag:"chunks"`
	FlashAttn          bool     `flag:"flash-attn,fa"`
	Prompt             *string  `flag:"prompt,p"`
	File               *string  `flag:"file,f"`
	InFile             []string `flag:"in-file"`
	BinaryFile         *string  `flag:"binary-file,bf"`
	Escape             *bool    `flag:"escape,e"`
	PrintTokenCount    *int     `flag:"print-token-count,ptc"`
	PromptCache        *string  `flag:"prompt-cache"`
	PromptCacheAll     bool     `flag:"prompt-cache-all"`
	PromptCacheRO      bool     `flag:"prompt-cache-ro"`
	ReversePrompt      []string `flag:"reverse-prompt,r"`
	Special            bool     `flag:"special,sp"`
	Conversation       bool     `flag:"conversation,cnv"`
	Interactive        bool     `flag:"interactive,i"`
	InteractiveFirst   bool     `flag:"interactive-first,if"`
	MultilineInput     bool     `flag:"multiline-input,mli"`
	InPrefixBos        bool     `flag:"in-prefix-bos"`
	InPrefix           *string  `flag:"in-prefix"`
	InSuffix           *string  `flag:"in-suffix"`
	SpmInfill          bool     `flag:"spm-infill"`
}

type SamplingOptions struct {
	Samplers              *string  `flag:"samplers"`
	SamplingSeq           *string  `flag:"sampling-seq"`
	IgnoreEos             bool     `flag:"ignore-eos"`
	PenalizeNl            bool     `flag:"penalize-nl"`
	Temp                  *float64 `flag:"temp"`
	TopK                  *int     `flag:"top-k"`
	TopP                  *float64 `flag:"top-p"`
	MinP                  *float64 `flag:"min-p"`
	Tfs                   *float64 `flag:"tfs"`
	Typical               *float64 `flag:"typical"`
	RepeatLastN           *int     `flag:"repeat-last-n"`
	RepeatPenalty         *float64 `flag:"repeat-penalty"`
	PresencePenalty       *float64 `flag:"presence-penalty"`
	FrequencyPenalty      *float64 `flag:"frequency-penalty"`
	DynatempRange         *float64 `flag:"dynatemp-range"`
	DynatempExp           *float64 `flag:"dynatemp-exp"`
	Mirostat              *int     `flag:"mirostat"`
	MirostatLr            *float64 `flag:"mirostat-lr"`
	MirostatEnt           *float64 `flag:"mirostat-ent"`
	LogitBias             []string `flag:"logit-bias,l"`
	CfgNegativePrompt     *string  `flag:"cfg-negative-prompt"`
	CfgNegativePromptFile *string  `flag:"cfg-negative-prompt-file"`
	CfgScale              *float64 `flag:"cfg-scale"`
	ChatTemplate          *string  `flag:"chat-template"`
}

type GrammarOptions struct {
	Grammar     *string `flag:"grammar"`
	GrammarFile *string `flag:"grammar-file"`
	JsonSchema  *string `flag:"json-schema,j"`
}

type EmbeddingOptions struct {
	Pooling          *string `flag:"pooling"`
	Attention        *string `flag:"attention"`
	EmbdNormalize    *int    `flag:"embd-normalize"`
	EmbdOutputFormat *string `flag:"embd-output-format"`
	EmbdSeparator    *string `flag:"embd-separator"`
}

type ContextOptions struct {
	RopeScaling    *string  `flag:"rope-scaling"`
	RopeScale      *float64 `flag:"rope-scale"`
	RopeFreqBase   *float64 `flag:"rope-freq-base"`
	RopeFreqScale  *float64 `flag:"rope-freq-scale"`
	YarnOrigCtx    *int     `flag:"yarn-orig-ctx"`
	YarnExtFactor  *float64 `flag:"yarn-ext-factor"`
	YarnAttnFactor *float64 `flag:"yarn-attn-factor"`
	YarnBetaSlow   *float64 `flag:"yarn-beta-slow"`
	YarnBetaFast   *float64 `flag:"yarn-beta-fast"`
	GrpAttnN       *int     `flag:"grp-attn-n,gan"`
	GrpAttnW       *float64 `flag:"grp-attn-w,gaw"`
	DumpKvCache    bool     `flag:"dump-kv-cache,dkvc"`
	NoKvOffload    bool     `flag:"no-kv-offload,nkvo"`
	CacheTypeK     *string  `flag:"cache-type-k,ctk"`
	CacheTypeV     *string  `flag:"cache-type-v,ctv"`
}

type ModelOptions struct {
	CheckTensors            bool     `flag:"check-tensors"`
	OverrideKv              []string `flag:"override-kv"`
	Lora                    *string  `flag:"lora"`
	LoraScaled              []string `flag:"lora-scaled"`
	LoraBase                *string  `flag:"lora-base"`
	ControlVector           []string `flag:"control-vector"`
	ControlVectorScaled     []string `flag:"control-vector-scaled"`
	ControlVectorLayerRange []int    `flag:"control-vector-layer-range"`
	Model                   *string  `flag:"model,m"`
	ModelDraft              *string  `flag:"model-draft,md"`
	ModelUrl                *string  `flag:"model-url,mu"`
	HfRepo                  *string  `flag:"hf-repo,hfr"`
	HfFile                  *string  `flag:"hf-file,hff"`
	HfToken                 *string  `flag:"hf-token,hft"`
}

type RetrievalOptions struct {
	ContextFile    []string `flag:"context-file"`
	ChunkSize      *int     `flag:"chunk-size"`
	ChunkSeparator *string  `flag:"chunk-separator"`
}

type PasskeyOptions struct {
	Junk *int `flag:"junk"`
	Pos  *int `flag:"pos"`
}

type IMatrixOptions struct {
	Output          *string `flag:"output,o"`
	OutputFrequency *int    `flag:"output-frequency"`
	SaveFrequency   *int    `flag:"save-frequency"`
	ProcessOutput   bool    `flag:"process-output"`
	NoPpl           bool    `flag:"no-ppl"`
	Chunk           *int    `flag:"chunk"`
}

type BenchOptions struct {
	Pps bool  `flag:"pps"`
	Npp []int `flag:"npp"`
	Ntg []int `flag:"ntg"`
	Npl []int `flag:"npl"`
}

type ServerOptions struct {
	Host                 *string  `flag:"host"`
	Port                 *int     `flag:"port"`
	Path                 *string  `flag:"path"`
	Embedding            bool     `flag:"embedding"`
	ApiKey               *string  `flag:"api-key"`
	ApiKeyFile           *string  `flag:"api-key-file"`
	SslKeyFile           *string  `flag:"ssl-key-file"`
	SslCertFile          *string  `flag:"ssl-cert-file"`
	Timeout              *int     `flag:"timeout"`
	ThreadsHttp          *int     `flag:"threads-http"`
	SystemPromptFile     *string  `flag:"system-prompt-file"`
	LogFormat            *string  `flag:"log-format"`
	Metrics              bool     `flag:"metrics"`
	NoSlots              bool     `flag:"no-slots"`
	SlotSavePath         *string  `flag:"slot-save-path"`
	ChatTemplate         *string  `flag:"chat-template"`
	SlotPromptSimilarity *float64 `flag:"slot-prompt-similarity,sps"`
}

type LoggingOptions struct {
	SimpleIo   bool    `flag:"simple-io"`
	Logdir     *string `flag:"logdir,ld"`
	LogTest    bool    `flag:"log-test"`
	LogDisable bool    `flag:"log-disable"`
	LogEnable  bool    `flag:"log-enable"`
	LogFile    *string `flag:"log-file"`
	LogNew     bool    `flag:"log-new"`
	LogAppend  bool    `flag:"log-append"`
}

type CVectorOptions struct {
	Output       *string `flag:"output,o"`
	PositiveFile *string `flag:"positive-file"`
	NegativeFile *string `flag:"negative-file"`
	PcaBatch     *int    `flag:"pca-batch"`
	PcaIter      *int    `flag:"pca-iter"`
	Method       *string `flag:"method"`
}
