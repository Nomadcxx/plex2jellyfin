using MediaBrowser.Common.Configuration;
using MediaBrowser.Common.Plugins;
using MediaBrowser.Model.Plugins;
using MediaBrowser.Model.Serialization;
using Plex2Jellyfin.Plugin.Configuration;

namespace Plex2Jellyfin.Plugin;

/// <summary>
/// Main entry point for the Plex2Jellyfin plugin.
/// Provides configuration page and plugin metadata.
/// </summary>
public class Plex2JellyfinPlugin : BasePlugin<PluginConfiguration>, IHasWebPages
{
    private const string ConfigPageResourcePath = "Plex2Jellyfin.Plugin.Configuration.configPage.html";

    /// <summary>
    /// Unique identifier for this plugin.
    /// Guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890
    /// </summary>
    public override Guid Id => Guid.Parse("a1b2c3d4-e5f6-7890-abcd-ef1234567890");

    /// <summary>
    /// Plugin name displayed in Jellyfin dashboard.
    /// </summary>
    public override string Name => "Plex2Jellyfin";

    /// <summary>
    /// Plugin description displayed in Jellyfin dashboard.
    /// </summary>
    public override string Description => "Companion plugin for Plex2Jellyfin media organizer - provides custom endpoints and event forwarding";

    /// <summary>
    /// Static instance reference for accessing plugin from other components.
    /// </summary>
    public static Plex2JellyfinPlugin Instance { get; private set; } = null!;

    /// <summary>
    /// Initializes a new instance of the <see cref="Plex2JellyfinPlugin"> class.
    /// </summary>
    /// <param name="applicationPaths">The application paths.</param>
    /// <param name="xmlSerializer">The XML serializer.</param>
    public Plex2JellyfinPlugin(IApplicationPaths applicationPaths, IXmlSerializer xmlSerializer)
        : base(applicationPaths, xmlSerializer)
    {
        Instance = this;
    }

    /// <summary>
    /// Gets the plugin configuration page for the Jellyfin dashboard.
    /// </summary>
    /// <returns>Plugin configuration pages.</returns>
    public IEnumerable<PluginPageInfo> GetPages()
    {
        return new[]
        {
            new PluginPageInfo
            {
                Name = "Plex2Jellyfin",
                EmbeddedResourcePath = ConfigPageResourcePath
            }
        };
    }
}
