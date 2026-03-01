using MediaBrowser.Controller.Entities;
using MediaBrowser.Controller.Library;
using MediaBrowser.Controller.Session;
using MediaBrowser.Model.Entities;
using Microsoft.AspNetCore.Authorization;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Mvc;
using Microsoft.Extensions.Logging;

namespace JellyWatch.Plugin.Api;

[ApiController]
[Route("jellywatch")]
[Authorize]
public class JellyWatchController : ControllerBase
{
    private static readonly object PauseStateLock = new();
    private static DateTime? _scanPausedUntilUtc;
    private static string? _scanPauseReason;

    private readonly ILibraryManager _libraryManager;
    private readonly ISessionManager _sessionManager;
    private readonly ILogger<JellyWatchController> _logger;

    public JellyWatchController(
        ILibraryManager libraryManager,
        ISessionManager sessionManager,
        ILogger<JellyWatchController> logger)
    {
        _libraryManager = libraryManager;
        _sessionManager = sessionManager;
        _logger = logger;
    }

    [HttpPost("refresh-path")]
    [ProducesResponseType(StatusCodes.Status202Accepted)]
    [ProducesResponseType(StatusCodes.Status400BadRequest)]
    [ProducesResponseType(StatusCodes.Status404NotFound)]
    [ProducesResponseType(StatusCodes.Status409Conflict)]
    public async Task<IActionResult> RefreshPath([FromBody] RefreshPathRequest request)
    {
        if (request == null || string.IsNullOrWhiteSpace(request.Path))
        {
            return BadRequest(new { Error = "Path is required" });
        }

        var pauseState = GetPauseState();
        if (pauseState.IsPaused)
        {
            return Conflict(new
            {
                Error = "Scanning is currently paused",
                PauseUntilUtc = pauseState.PauseUntilUtc?.ToString("O")
            });
        }

        var folder = GetFolderByPath(request.Path);
        if (folder == null)
        {
            return NotFound(new { Error = "Library not found for path", Path = request.Path });
        }

        await _libraryManager.ValidateMediaLibrary(new Progress<double>(), CancellationToken.None);
        _logger.LogInformation("Refresh triggered for path {Path} in library {LibraryName}", request.Path, folder.Name);

        return Accepted(new
        {
            Message = "Refresh started",
            LibraryName = folder.Name,
            Path = request.Path
        });
    }

    [HttpPost("pause-scanning")]
    [ProducesResponseType(StatusCodes.Status200OK)]
    public IActionResult PauseScanning([FromBody] PauseScanningRequest? request)
    {
        var requestedMinutes = request?.DurationMinutes ?? 15;
        var durationMinutes = Math.Clamp(requestedMinutes, 1, 240);
        var pauseUntilUtc = DateTime.UtcNow.AddMinutes(durationMinutes);

        lock (PauseStateLock)
        {
            _scanPausedUntilUtc = pauseUntilUtc;
            _scanPauseReason = request?.Reason;
        }

        _logger.LogInformation("Scanning paused until {PauseUntilUtc}", pauseUntilUtc);

        return Ok(new
        {
            IsPaused = true,
            PauseUntilUtc = pauseUntilUtc.ToString("O"),
            DurationMinutes = durationMinutes,
            Reason = request?.Reason
        });
    }

    [HttpPost("resume-scanning")]
    [ProducesResponseType(StatusCodes.Status200OK)]
    public IActionResult ResumeScanning()
    {
        lock (PauseStateLock)
        {
            _scanPausedUntilUtc = null;
            _scanPauseReason = null;
        }

        _logger.LogInformation("Scanning resumed");
        return Ok(new { IsPaused = false });
    }

    [HttpGet("identification/{itemId}")]
    [ProducesResponseType(StatusCodes.Status200OK)]
    [ProducesResponseType(StatusCodes.Status404NotFound)]
    public IActionResult GetIdentification([FromRoute] string itemId)
    {
        if (string.IsNullOrWhiteSpace(itemId))
        {
            return BadRequest(new { Error = "ItemId is required" });
        }

        var item = _libraryManager.RootFolder
            .GetRecursiveChildren(i => i.Id.ToString().Equals(itemId, StringComparison.OrdinalIgnoreCase))
            .FirstOrDefault();

        if (item == null)
        {
            return NotFound(new { Error = "Item not found", ItemId = itemId });
        }

        return Ok(new
        {
            ItemId = item.Id.ToString(),
            Name = item.Name,
            Path = item.Path,
            Type = item.GetType().Name,
            ProviderIds = item.ProviderIds,
            IsIdentified = item.ProviderIds.Count > 0,
            ProviderCount = item.ProviderIds.Count,
            LastUpdatedUtc = item.DateModified.ToString("O")
        });
    }

    [HttpGet("status")]
    [AllowAnonymous]
    [ProducesResponseType(StatusCodes.Status200OK)]
    public IActionResult GetStatus()
    {
        var pauseState = GetPauseState();
        var config = JellyWatchPlugin.Instance?.Configuration;

        return Ok(new
        {
            PluginVersion = typeof(JellyWatchPlugin).Assembly.GetName().Version?.ToString() ?? "unknown",
            EndpointsEnabled = config?.EnableCustomEndpoints ?? true,
            EventForwardingEnabled = config?.EnableEventForwarding ?? false,
            JellyWatchUrl = config?.JellyWatchUrl,
            ScanningPaused = pauseState.IsPaused,
            ScanPauseUntilUtc = pauseState.PauseUntilUtc?.ToString("O"),
            ScanPauseReason = pauseState.Reason,
            Timestamp = DateTime.UtcNow.ToString("O")
        });
    }

    [HttpGet("health")]
    [AllowAnonymous]
    [ProducesResponseType(StatusCodes.Status200OK)]
    public IActionResult Health()
    {
        return Ok(new
        {
            Status = "healthy",
            PluginVersion = "1.0.0",
            Timestamp = DateTime.UtcNow.ToString("O"),
            Libraries = _libraryManager.GetVirtualFolders().Count()
        });
    }

    [HttpGet("item-by-path")]
    [ProducesResponseType(StatusCodes.Status200OK)]
    [ProducesResponseType(StatusCodes.Status404NotFound)]
    public IActionResult GetItemByPath([FromQuery] string path)
    {
        if (string.IsNullOrWhiteSpace(path))
        {
            return BadRequest(new { Error = "Path is required" });
        }

        var item = _libraryManager.RootFolder
            .GetRecursiveChildren(i => !string.IsNullOrEmpty(i.Path) && i.Path.Equals(path, StringComparison.OrdinalIgnoreCase))
            .FirstOrDefault();

        if (item == null)
        {
            return NotFound(new { Error = "Item not found", Path = path });
        }

        return Ok(BuildItemResponse(item));
    }

    [HttpGet("library-by-path")]
    [ProducesResponseType(StatusCodes.Status200OK)]
    [ProducesResponseType(StatusCodes.Status404NotFound)]
    public IActionResult GetLibraryByPath([FromQuery] string path)
    {
        if (string.IsNullOrWhiteSpace(path))
        {
            return BadRequest(new { Error = "Path is required" });
        }

        var folder = GetFolderByPath(path);

        if (folder == null)
        {
            return NotFound(new { Error = "Library not found for path", Path = path });
        }

        return Ok(new
        {
            LibraryId = folder.ItemId,
            Name = folder.Name,
            CollectionType = folder.CollectionType,
            Locations = folder.Locations,
            Path = path
        });
    }

    [HttpGet("active-scans")]
    [ProducesResponseType(StatusCodes.Status200OK)]
    public IActionResult GetActiveScans()
    {
        return Ok(new
        {
            ActiveScanCount = 0,
            Scans = Array.Empty<object>()
        });
    }

    [HttpPost("scan-library")]
    [ProducesResponseType(StatusCodes.Status202Accepted)]
    [ProducesResponseType(StatusCodes.Status404NotFound)]
    public async Task<IActionResult> ScanLibrary([FromBody] ScanRequest request)
    {
        if (GetPauseState().IsPaused)
        {
            return Conflict(new { Error = "Scanning is currently paused" });
        }

        if (string.IsNullOrWhiteSpace(request.LibraryId) && string.IsNullOrWhiteSpace(request.Path))
        {
            return BadRequest(new { Error = "LibraryId or Path is required" });
        }

        var folder = string.IsNullOrWhiteSpace(request.LibraryId)
            ? GetFolderByPath(request.Path!)
            : _libraryManager.GetVirtualFolders()
                .FirstOrDefault(f => f.ItemId == request.LibraryId);

        if (folder == null)
        {
            return NotFound(new { Error = "Library not found" });
        }

        await _libraryManager.ValidateMediaLibrary(new Progress<double>(), CancellationToken.None);
        _logger.LogInformation("Scan triggered for library {LibraryName} by JellyWatch", folder.Name);

        return Accepted(new { Message = "Scan started", LibraryName = folder.Name });
    }

    [HttpGet("unidentifiable")]
    [ProducesResponseType(StatusCodes.Status200OK)]
    public IActionResult GetUnidentifiableItems([FromQuery] string? libraryId = null)
    {
        var allItems = _libraryManager.RootFolder.GetRecursiveChildren(i => i.ProviderIds.Count == 0);

        IEnumerable<BaseItem> filtered = allItems;

        if (!string.IsNullOrWhiteSpace(libraryId))
        {
            var folder = _libraryManager.GetVirtualFolders().FirstOrDefault(f => f.ItemId == libraryId);
            if (folder != null)
            {
                filtered = allItems.Where(i => folder.Locations.Any(l =>
                    i.Path?.StartsWith(l, StringComparison.OrdinalIgnoreCase) == true));
            }
        }

        var items = filtered
            .Select(item => new
            {
                Id = item.Id.ToString(),
                Name = item.Name,
                Path = item.Path,
                Type = item.GetType().Name,
                DateAdded = item.DateCreated.ToString("O")
            })
            .Take(100)
            .ToList();

        return Ok(new
        {
            TotalCount = items.Count,
            Items = items
        });
    }

    [HttpGet("active-playback")]
    [ProducesResponseType(StatusCodes.Status200OK)]
    public IActionResult GetActivePlayback()
    {
        var sessions = _sessionManager.Sessions
            .Where(s => s.NowPlayingItem != null)
            .Select(s => new
            {
                SessionId = s.Id,
                UserId = s.UserId.ToString(),
                UserName = s.UserName,
                DeviceName = s.DeviceName,
                Client = s.Client,
                ItemName = s.NowPlayingItem?.Name,
                ItemType = s.NowPlayingItem?.Type,
                PositionTicks = s.PlayState?.PositionTicks,
                IsPaused = s.PlayState?.IsPaused
            })
            .ToList();

        return Ok(new
        {
            ActiveSessionCount = sessions.Count,
            Sessions = sessions
        });
    }

    private (bool IsPaused, DateTime? PauseUntilUtc, string? Reason) GetPauseState()
    {
        lock (PauseStateLock)
        {
            if (_scanPausedUntilUtc.HasValue && _scanPausedUntilUtc > DateTime.UtcNow)
            {
                return (true, _scanPausedUntilUtc, _scanPauseReason);
            }

            if (_scanPausedUntilUtc.HasValue)
            {
                _scanPausedUntilUtc = null;
                _scanPauseReason = null;
            }

            return (false, null, null);
        }
    }

    private VirtualFolderInfo? GetFolderByPath(string path)
    {
        return _libraryManager.GetVirtualFolders()
            .FirstOrDefault(f => f.Locations.Any(l => path.StartsWith(l, StringComparison.OrdinalIgnoreCase)));
    }

    private static object BuildItemResponse(BaseItem item)
    {
        return new
        {
            Id = item.Id.ToString(),
            Name = item.Name,
            Path = item.Path,
            Type = item.GetType().Name,
            ProviderIds = item.ProviderIds,
            IsIdentified = item.ProviderIds.Count > 0,
            Overview = item.Overview,
            Tags = item.Tags,
            Genres = item.Genres,
            Studios = item.Studios,
            ProductionYear = item.ProductionYear,
            ParentalRating = item.OfficialRating,
            CommunityRating = item.CommunityRating,
            DateCreated = item.DateCreated.ToString("O"),
            DateModified = item.DateModified.ToString("O"),
            RunTimeTicks = item.RunTimeTicks,
            Container = item.Container
        };
    }
}

public class ScanRequest
{
    public string? LibraryId { get; set; }
    public string? Path { get; set; }
}

public class RefreshPathRequest
{
    public string? Path { get; set; }
}

public class PauseScanningRequest
{
    public int DurationMinutes { get; set; } = 15;
    public string? Reason { get; set; }
}
