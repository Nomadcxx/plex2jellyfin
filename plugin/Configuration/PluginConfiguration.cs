using MediaBrowser.Model.Plugins;

namespace Plex2Jellyfin.Plugin.Configuration;

/// <summary>
/// Plugin configuration for Plex2Jellyfin.
/// </summary>
public class PluginConfiguration : BasePluginConfiguration
{
    /// <summary>
    /// Plex2Jellyfin daemon URL (e.g., http://localhost:3000).
    /// </summary>
    public string Plex2JellyfinUrl { get; set; } = "http://localhost:3000";

    /// <summary>
    /// Shared secret for webhook authentication.
    /// </summary>
    public string SharedSecret { get; set; } = "";

    /// <summary>
    /// Enable event forwarding to Plex2Jellyfin.
    /// </summary>
    public bool EnableEventForwarding { get; set; } = true;

    /// <summary>
    /// Enable custom API endpoints.
    /// </summary>
    public bool EnableCustomEndpoints { get; set; } = true;

    /// <summary>
    /// Timeout for HTTP requests to Plex2Jellyfin (in seconds).
    /// </summary>
    public int RequestTimeoutSeconds { get; set; } = 30;

    /// <summary>
    /// Retry count for failed requests.
    /// </summary>
    public int RetryCount { get; set; } = 3;

    /// <summary>
    /// Forward playback events.
    /// </summary>
    public bool ForwardPlaybackEvents { get; set; } = true;

    /// <summary>
    /// Forward library events (item added/removed/updated).
    /// </summary>
    public bool ForwardLibraryEvents { get; set; } = true;
}
