using JellyWatch.Plugin.Api;
using JellyWatch.Plugin.EventHandlers;
using MediaBrowser.Controller;
using MediaBrowser.Controller.Plugins;
using Microsoft.Extensions.DependencyInjection;

namespace JellyWatch.Plugin;

/// <summary>
/// Registers plugin services with the Jellyfin DI container.
/// </summary>
public class ServiceRegistrator : IPluginServiceRegistrator
{
    /// <summary>
    /// Registers services required by the plugin.
    /// </summary>
    public void RegisterServices(IServiceCollection serviceCollection, IServerApplicationHost applicationHost)
    {
        // Register event forwarder as a hosted service (replaces IServerEntryPoint for Jellyfin 10.10+).
        serviceCollection.AddHostedService<EventForwarder>();

        // Register HTTP client factory for event forwarding
        serviceCollection.AddHttpClient();

        // Register custom API controller
        serviceCollection.AddScoped<JellyWatchController>();
    }
}
