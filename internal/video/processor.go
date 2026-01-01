package video

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Processor handles video file processing and audio extraction
type Processor struct {
	TempDir string
}

// NewProcessor creates a new video processor
func NewProcessor(tempDir string) *Processor {
	return &Processor{
		TempDir: tempDir,
	}
}

// ExtractAudioResult contains the extracted audio data and metadata
type ExtractAudioResult struct {
	AudioData  []byte
	SampleRate int
	Channels   int
	Duration   float64
}

// ExtractAudio extracts audio from a video file and returns WAV data
// The audio is converted to 16-bit PCM, mono, 16kHz (optimal for Whisper)
func (p *Processor) ExtractAudio(videoPath string) (*ExtractAudioResult, error) {
	// Create temp file for extracted audio
	tempAudio := filepath.Join(p.TempDir, fmt.Sprintf("audio_%s.wav", filepath.Base(videoPath)))
	defer os.Remove(tempAudio)

	// Use ffmpeg to extract audio and convert to 16kHz mono 16-bit PCM
	cmd := exec.Command("ffmpeg",
		"-i", videoPath,
		"-vn",                  // No video
		"-acodec", "pcm_s16le", // 16-bit PCM
		"-ar", "16000", // 16kHz sample rate (Whisper optimal)
		"-ac", "1", // Mono
		"-y", // Overwrite output file
		tempAudio,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg error: %w, stderr: %s", err, stderr.String())
	}

	// Read the extracted audio file
	audioData, err := os.ReadFile(tempAudio)
	if err != nil {
		return nil, fmt.Errorf("read audio file: %w", err)
	}

	// Get duration using ffprobe
	duration, err := p.getVideoDuration(videoPath)
	if err != nil {
		// Non-critical, set to 0 if we can't get it
		duration = 0
	}

	return &ExtractAudioResult{
		AudioData:  audioData,
		SampleRate: 16000,
		Channels:   1,
		Duration:   duration,
	}, nil
}

// getVideoDuration uses ffprobe to get the video duration in seconds
func (p *Processor) getVideoDuration(videoPath string) (float64, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		videoPath,
	)

	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return 0, err
	}

	var duration float64
	if _, err := fmt.Sscanf(out.String(), "%f", &duration); err != nil {
		return 0, err
	}

	return duration, nil
}

// ReplaceAudio replaces the audio track in a video with new audio
// audioData should be MP3 audio bytes
// Returns the path to the output video file (caller must delete it)
func (p *Processor) ReplaceAudio(videoPath string, audioData []byte) (string, error) {
	// Save audio data to temp file
	tempAudio := filepath.Join(p.TempDir, fmt.Sprintf("tts_audio_%d.mp3", os.Getpid()))
	defer os.Remove(tempAudio)

	if err := os.WriteFile(tempAudio, audioData, 0644); err != nil {
		return "", fmt.Errorf("write audio file: %w", err)
	}

	// Create output video path - always output as MP4 for compatibility
	baseNameWithoutExt := filepath.Base(videoPath)
	// Remove extension
	if idx := strings.LastIndex(baseNameWithoutExt, "."); idx != -1 {
		baseNameWithoutExt = baseNameWithoutExt[:idx]
	}
	outputVideo := filepath.Join(p.TempDir, fmt.Sprintf("translated_%d_%s.mp4", os.Getpid(), baseNameWithoutExt))

	// Get original video duration
	videoDuration, err := p.getVideoDuration(videoPath)
	if err != nil {
		return "", fmt.Errorf("get video duration: %w", err)
	}

	// Get TTS audio duration
	audioDuration, err := p.getAudioDuration(tempAudio)
	if err != nil {
		return "", fmt.Errorf("get audio duration: %w", err)
	}

	// Use ffmpeg to replace audio
	// If audio is shorter than video, loop it; if longer, trim it
	var cmd *exec.Cmd
	if audioDuration < videoDuration {
		// Audio is shorter - loop it to match video duration
		cmd = exec.Command("ffmpeg",
			"-i", videoPath,
			"-stream_loop", "-1", // Loop audio indefinitely
			"-i", tempAudio,
			"-map", "0:v:0", // Use video from first input
			"-map", "1:a:0", // Use audio from second input
			"-c:v", "libx264", // Re-encode video to H.264 for MP4
			"-c:a", "aac", // Encode audio to AAC
			"-preset", "fast", // Fast encoding preset
			"-crf", "23", // Quality setting (lower = better quality, 23 is default)
			"-shortest", // End when shortest stream ends (video)
			"-y",
			outputVideo,
		)
	} else {
		// Audio is longer or equal - just combine and trim if needed
		cmd = exec.Command("ffmpeg",
			"-i", videoPath,
			"-i", tempAudio,
			"-map", "0:v:0", // Use video from first input
			"-map", "1:a:0", // Use audio from second input
			"-c:v", "libx264", // Re-encode video to H.264 for MP4
			"-c:a", "aac", // Encode audio to AAC
			"-preset", "fast", // Fast encoding preset
			"-crf", "23", // Quality setting
			"-shortest", // End when video ends
			"-y",
			outputVideo,
		)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffmpeg error: %w, stderr: %s", err, stderr.String())
	}

	return outputVideo, nil
}

// getAudioDuration gets the duration of an audio file in seconds
func (p *Processor) getAudioDuration(audioPath string) (float64, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		audioPath,
	)

	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return 0, err
	}

	var duration float64
	if _, err := fmt.Sscanf(out.String(), "%f", &duration); err != nil {
		return 0, err
	}

	return duration, nil
}

// ConvertAudioToWAV converts any audio file to WAV format (16kHz mono 16-bit PCM)
func (p *Processor) ConvertAudioToWAV(audioPath string) (*ExtractAudioResult, error) {
	// Create temp file for converted audio
	tempWAV := filepath.Join(p.TempDir, fmt.Sprintf("converted_%s.wav", filepath.Base(audioPath)))
	defer os.Remove(tempWAV)

	// Use ffmpeg to convert audio to 16kHz mono 16-bit PCM WAV
	cmd := exec.Command("ffmpeg",
		"-i", audioPath,
		"-acodec", "pcm_s16le", // 16-bit PCM
		"-ar", "16000",         // 16kHz sample rate (Whisper optimal)
		"-ac", "1",             // Mono
		"-y",                   // Overwrite output file
		tempWAV,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg error: %w, stderr: %s", err, stderr.String())
	}

	// Read the converted audio file
	audioData, err := os.ReadFile(tempWAV)
	if err != nil {
		return nil, fmt.Errorf("read audio file: %w", err)
	}

	// Get duration using ffprobe
	duration, err := p.getAudioDuration(audioPath)
	if err != nil {
		// Non-critical, set to 0 if we can't get it
		duration = 0
	}

	return &ExtractAudioResult{
		AudioData:  audioData,
		SampleRate: 16000,
		Channels:   1,
		Duration:   duration,
	}, nil
}

// ConvertAudioToWAVWithEnhancement converts audio to WAV and optionally applies noise reduction.
func (p *Processor) ConvertAudioToWAVWithEnhancement(audioPath string, enhance bool) (*ExtractAudioResult, error) {
	// Create temp file for converted audio
	tempWAV := filepath.Join(p.TempDir, fmt.Sprintf("converted_%s.wav", filepath.Base(audioPath)))
	defer os.Remove(tempWAV)

	args := []string{
		"-i", audioPath,
		"-acodec", "pcm_s16le", // 16-bit PCM
		"-ar", "16000",         // 16kHz sample rate (Whisper optimal)
		"-ac", "1",             // Mono
	}
	if enhance {
		// Light denoise + band-pass to emphasize speech
		args = append(args, "-af", "highpass=f=80,lowpass=f=8000,afftdn=nr=12")
	}
	args = append(args, "-y", tempWAV)

	cmd := exec.Command("ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg error: %w, stderr: %s", err, stderr.String())
	}

	// Read the converted audio file
	audioData, err := os.ReadFile(tempWAV)
	if err != nil {
		return nil, fmt.Errorf("read audio file: %w", err)
	}

	// Get duration using ffprobe
	duration, err := p.getAudioDuration(audioPath)
	if err != nil {
		// Non-critical, set to 0 if we can't get it
		duration = 0
	}

	return &ExtractAudioResult{
		AudioData:  audioData,
		SampleRate: 16000,
		Channels:   1,
		Duration:   duration,
	}, nil
}

// CheckFFmpegInstalled verifies that ffmpeg and ffprobe are available
func CheckFFmpegInstalled() error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg not found: %w", err)
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return fmt.Errorf("ffprobe not found: %w", err)
	}
	return nil
}
