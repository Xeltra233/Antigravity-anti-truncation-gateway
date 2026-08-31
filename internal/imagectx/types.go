package imagectx

// Mode represents the image context transformation mode.
type Mode string

const (
	// ModeStandard represents official standard text anti-truncation mode (no image rasterization).
	ModeStandard Mode = "standard"
	// ModeAllImage represents experimental full image stream mode (all context text rasterized into PNG images).
	ModeAllImage Mode = "all_image"
	// ModeCurrentTurnOnly represents hybrid experimental mode (only the latest/current user turn is rasterized into PNG images).
	ModeCurrentTurnOnly Mode = "current_turn_only"
)

// PipelineConfig contains the configuration parameters for the image context pipeline.
type PipelineConfig struct {
	Mode                 Mode   // standard, all_image, current_turn_only
	StandardPrefix       string // e.g. "[抗截断] "
	ExperimentalPrefix   string // e.g. "[实验性] "
	HybridPrefix         string // e.g. "[混合实验性] "
	MaxRunesPerPage      int    // default 1500
	MaxLinesPerPage      int    // default 40
	MaxPages             int    // default 100
	MaxTotalBytes        int64  // default 12582912 (12 MiB)
	MaxSingleBytes       int64  // default 4194304 (4 MiB)
	FallbackOnError      bool   // default true
	FontPath             string // optional custom font path
}

// DefaultPipelineConfig returns the default pipeline configuration.
func DefaultPipelineConfig() *PipelineConfig {
	return &PipelineConfig{
		Mode:               ModeStandard,
		StandardPrefix:     "[抗截断] ",
		ExperimentalPrefix: "[实验性] ",
		HybridPrefix:       "[混合实验性] ",
		MaxRunesPerPage:    1500,
		MaxLinesPerPage:    40,
		MaxPages:           100,
		MaxTotalBytes:      12582912, // 12 MiB
		MaxSingleBytes:     4194304,  // 4 MiB
		FallbackOnError:    true,
	}
}
