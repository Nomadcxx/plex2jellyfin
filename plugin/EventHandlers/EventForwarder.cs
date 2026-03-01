using System.Text;
using System.Text.Json;
using JellyWatch.Plugin.Configuration;
using MediaBrowser.Controller.Entities;
using MediaBrowser.Controller.Library;
using MediaBrowser.Controller.Session;
using MediaBrowser.Model.Entities;
using MediaBrowser.Model.Session;
using MediaBrowser.Model.Tasks;
using Microsoft.Extensions.Hosting;
using Microsoft.Extensions.Logging;

namespace JellyWatch.Plugin.EventHandlers;

public class EventForwarder : IHostedService, IDisposable
{
    private readonly ILibraryManager _libraryManager;
    private readonly ISessionManager _sessionManager;
    private readonly ITaskManager _taskManager;
    private readonly IMediaSourceManager _mediaSourceManager;
    private readonly IHttpClientFactory _httpClientFactory;
    private readonly ILogger<EventForwarder> _logger;

    private static DateTime _lastProgressEventSent = DateTime.MinValue;
    private bool _disposed;

    public EventForwarder(
        ILibraryManager libraryManager,
        ISessionManager sessionManager,
        ITaskManager taskManager,
        IMediaSourceManager mediaSourceManager,
        IHttpClientFactory httpClientFactory,
        ILogger<EventForwarder> logger)
    {
        _libraryManager = libraryManager;
        _sessionManager = sessionManager;
        _taskManager = taskManager;
        _mediaSourceManager = mediaSourceManager;
        _httpClientFactory = httpClientFactory;
        _logger = logger;
    }

    public Task StartAsync(CancellationToken cancellationToken)
    {
        var config = JellyWatchPlugin.Instance?.Configuration;
        if (config?.EnableEventForwarding != true)
        {
            _logger.LogInformation("Event forwarding is disabled");
            return Task.CompletedTask;
        }

        if (config.ForwardLibraryEvents)
        {
            _libraryManager.ItemAdded += OnItemAdded;
            _libraryManager.ItemRemoved += OnItemRemoved;
            _libraryManager.ItemUpdated += OnItemUpdated;
        }

        if (config.ForwardPlaybackEvents)
        {
            _sessionManager.PlaybackStart += OnPlaybackStart;
            _sessionManager.PlaybackStopped += OnPlaybackStopped;
            _sessionManager.PlaybackProgress += OnPlaybackProgress;
        }

        _taskManager.TaskCompleted += OnTaskCompleted;

        _logger.LogInformation("EventForwarder started");
        return Task.CompletedTask;
    }

    public Task StopAsync(CancellationToken cancellationToken)
    {
        _libraryManager.ItemAdded -= OnItemAdded;
        _libraryManager.ItemRemoved -= OnItemRemoved;
        _libraryManager.ItemUpdated -= OnItemUpdated;
        _sessionManager.PlaybackStart -= OnPlaybackStart;
        _sessionManager.PlaybackStopped -= OnPlaybackStopped;
        _sessionManager.PlaybackProgress -= OnPlaybackProgress;
        _taskManager.TaskCompleted -= OnTaskCompleted;
        return Task.CompletedTask;
    }

    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        StopAsync(CancellationToken.None).GetAwaiter().GetResult();
        GC.SuppressFinalize(this);
    }

    private async void OnItemAdded(object? sender, ItemChangeEventArgs e)
    {
        if (!ShouldForwardEvent()) return;
        await ForwardEvent("ItemAdded", BuildItemPayload(e.Item));
    }

    private async void OnItemRemoved(object? sender, ItemChangeEventArgs e)
    {
        if (!ShouldForwardEvent()) return;
        await ForwardEvent("ItemRemoved", BuildItemPayload(e.Item));
    }

    private async void OnItemUpdated(object? sender, ItemChangeEventArgs e)
    {
        if (!ShouldForwardEvent()) return;
        await ForwardEvent("ItemUpdated", BuildItemPayload(e.Item));
    }

    private async void OnPlaybackStart(object? sender, PlaybackProgressEventArgs e)
    {
        if (!ShouldForwardEvent()) return;
        await ForwardEvent("PlaybackStart", BuildPlaybackPayload(e));
    }

    private async void OnPlaybackStopped(object? sender, PlaybackProgressEventArgs e)
    {
        if (!ShouldForwardEvent()) return;
        await ForwardEvent("PlaybackStopped", BuildPlaybackPayload(e));
    }

    private async void OnPlaybackProgress(object? sender, PlaybackProgressEventArgs e)
    {
        if ((DateTime.UtcNow - _lastProgressEventSent).TotalSeconds < 30) return;
        if (!ShouldForwardEvent()) return;
        _lastProgressEventSent = DateTime.UtcNow;
        await ForwardEvent("PlaybackProgress", BuildPlaybackPayload(e));
    }

    private async void OnTaskCompleted(object? sender, TaskCompletionEventArgs e)
    {
        if (!ShouldForwardEvent()) return;
        await ForwardEvent("TaskCompleted", BuildTaskCompletedPayload(e));
    }

    private static bool ShouldForwardEvent()
    {
        return JellyWatchPlugin.Instance?.Configuration?.EnableEventForwarding == true;
    }

    private object BuildItemPayload(BaseItem item)
    {
        var hasSubtitles = false;
        try
        {
            var streams = _mediaSourceManager.GetMediaStreams(item.Id);
            hasSubtitles = streams.Any(s => s.Type == MediaStreamType.Subtitle);
        }
        catch (Exception ex)
        {
            _logger.LogDebug(ex, "Could not retrieve media streams for {ItemName}", item.Name);
        }

        return new
        {
            EventType = "ItemChanged",
            Timestamp = DateTime.UtcNow.ToString("O"),
            Item = new
            {
                Id = item.Id.ToString(),
                Name = item.Name,
                Path = item.Path,
                Type = item.GetType().Name,
                ProviderIds = item.ProviderIds,
                IsIdentified = item.ProviderIds.Count > 0,
                LibraryName = item.GetParent()?.Name,
                ParentId = item.ParentId.ToString(),
                HasSubtitles = hasSubtitles,
                PrimaryImagePath = item.GetImagePath(ImageType.Primary),
                DateCreated = item.DateCreated.ToString("O"),
                DateModified = item.DateModified.ToString("O")
            }
        };
    }

    private static object BuildPlaybackPayload(PlaybackProgressEventArgs e)
    {
        var item = e.Item;

        return new
        {
            EventType = "Playback",
            Timestamp = DateTime.UtcNow.ToString("O"),
            Session = new
            {
                Id = e.Session.Id,
                DeviceId = e.Session.DeviceId,
                DeviceName = e.Session.DeviceName,
                Client = e.Session.Client,
                UserId = e.Session.UserId.ToString(),
                UserName = e.Session.UserName
            },
            Item = item != null ? new
            {
                Id = item.Id.ToString(),
                Name = item.Name,
                Path = item.Path,
                Type = item.GetType().Name
            } : (object?)null,
            Playback = new
            {
                PositionTicks = e.PlaybackPositionTicks,
                DurationTicks = item?.RunTimeTicks,
                IsPaused = e.IsPaused
            }
        };
    }

    private static object BuildTaskCompletedPayload(TaskCompletionEventArgs e)
    {
        return new
        {
            EventType = "TaskCompleted",
            Timestamp = DateTime.UtcNow.ToString("O"),
            Task = new
            {
                Id = e.Task.Id.ToString(),
                Name = e.Task.Name,
                Category = e.Task.Category
            },
            Result = new
            {
                Status = e.Result.Status.ToString(),
                StartTimeUtc = e.Result.StartTimeUtc.ToString("O"),
                EndTimeUtc = e.Result.EndTimeUtc.ToString("O"),
                ErrorMessage = e.Result.ErrorMessage,
                LongErrorMessage = e.Result.LongErrorMessage
            }
        };
    }

    private async Task ForwardEvent(string eventType, object payload)
    {
        var config = JellyWatchPlugin.Instance?.Configuration;
        if (config == null) return;

        var url = $"{config.JellyWatchUrl.TrimEnd('/')}/api/v1/webhooks/jellyfin";
        var maxRetries = config.RetryCount;
        var timeout = TimeSpan.FromSeconds(config.RequestTimeoutSeconds);

        var requestPayload = new
        {
            EventType = eventType,
            Timestamp = DateTime.UtcNow.ToString("O"),
            Payload = payload
        };

        for (int attempt = 0; attempt < maxRetries; attempt++)
        {
            try
            {
                using var client = _httpClientFactory.CreateClient();
                client.Timeout = timeout;

                var json = JsonSerializer.Serialize(requestPayload, new JsonSerializerOptions
                {
                    PropertyNamingPolicy = JsonNamingPolicy.CamelCase
                });

                var request = new HttpRequestMessage(HttpMethod.Post, url)
                {
                    Content = new StringContent(json, Encoding.UTF8, "application/json")
                };

                request.Headers.Add("X-Jellywatch-Webhook-Secret", config.SharedSecret);
                request.Headers.Add("X-Jellyfin-Event", eventType);

                var response = await client.SendAsync(request);
                if (response.IsSuccessStatusCode)
                {
                    _logger.LogDebug("Forwarded {EventType} event to JellyWatch", eventType);
                    return;
                }

                _logger.LogWarning("Failed to forward {EventType} event: {StatusCode}",
                    eventType, response.StatusCode);
            }
            catch (Exception ex)
            {
                _logger.LogWarning(ex, "Error forwarding {EventType} event (attempt {Attempt}/{Max})",
                    eventType, attempt + 1, maxRetries);
            }

            if (attempt < maxRetries - 1)
            {
                await Task.Delay(TimeSpan.FromSeconds(Math.Pow(2, attempt)));
            }
        }

        _logger.LogError("Failed to forward {EventType} event after {MaxRetries} attempts",
            eventType, maxRetries);
    }
}
