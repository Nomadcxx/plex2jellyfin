# JellyWatch Architecture

```mermaid
graph TB
    %% --- STYLING ---
    classDef service fill:#20283e,stroke:#657b83,stroke-width:2px,color:#fff;
    classDef storage fill:#3d3418,stroke:#b58900,stroke-width:2px,color:#fff,shape:cylinder;
    classDef logic fill:#183d22,stroke:#2aa198,stroke-width:2px,color:#fff;
    classDef file fill:#2e3440,stroke:#839496,stroke-width:1px,color:#fff,stroke-dasharray: 5 5;
    classDef decision fill:#4c1818,stroke:#dc322f,stroke-width:2px,color:#fff;

    %% --- ZONE 1: ARR STACK (Sources) ---
    subgraph ArrStack ["Arr Stack"]
        direction LR
        Sonarr[Sonarr]:::service
        Radarr[Radarr]:::service
    end

    %% --- ZONE 2: INGESTION PIPELINE ---
    Downloader[Download Client<br/>qBit/Transmission/SAB]:::service
    WatchDir[/Watch Directory<br/>/downloads/]:::file

    Sonarr -- "sends TV episode" --> Downloader
    Radarr -- "sends movie" --> Downloader
    Downloader -- "drops file" --> WatchDir

    %% --- ZONE 3: JELLYWATCH CORE ---
    subgraph JellyWatchCore ["JellyWatch Core"]
        direction TB
        Watcher[File Watcher]:::logic
        Parser[Name Parser]:::logic
        Decision{confidence<br/>below 0.8?}:::decision
        AI[AI Resolver<br/>Ollama llama3.1]:::logic
        Organizer[Organizer]:::logic
        
        WatchDir -- "file detected" --> Watcher
        Watcher --> Parser
        Parser --> Decision
        Decision -- "yes" --> AI
        Decision -- "no" --> Organizer
        AI --> Organizer
    end

    %% --- API METADATA LOOPS ---
    Organizer -.->|"query series info"| Sonarr
    Organizer -.->|"query movie info"| Radarr

    %% --- ZONE 4: PERSISTENCE ---
    JWDB[(JellyWatch DB<br/>SQLite)]:::storage
    
    subgraph Library ["Jellyfin Library"]
        direction LR
        Drive1[/"Drive 1<br/>/mnt/disk1"/]:::file
        Drive2[/"Drive 2<br/>/mnt/disk2"/]:::file
        Drive3[/"Drive 3<br/>/mnt/disk3"/]:::file
    end

    Organizer -->|"index, store confidence,<br/>track location"| JWDB
    Organizer -->|"rename, move,<br/>set permissions"| Library

    %% --- ZONE 5: MAINTENANCE (CLI Commands) ---
    subgraph Maintenance ["Library Maintenance (CLI)"]
        direction LR
        Scan[scan]:::logic
        Audit[audit]:::logic
        Dedupe[duplicates]:::logic
        Consolidate[consolidate]:::logic
    end

    %% Maintenance reads from DB and operates on Library
    Scan -->|"re-index"| JWDB
    Audit -->|"low confidence"| JWDB
    Dedupe -->|"find dupes"| JWDB
    Consolidate -->|"scattered series"| JWDB
    
    JWDB -.->|"update locations"| Library
    Library -.->|"scan & serve"| Jellyfin[Jellyfin Server]:::service

    %% Style the subgraph titles
    style ArrStack fill:none,stroke:#586e75,stroke-width:1px,color:#839496
    style JellyWatchCore fill:none,stroke:#2aa198,stroke-width:1px,color:#839496
    style Library fill:none,stroke:#b58900,stroke-width:1px,color:#839496
    style Maintenance fill:none,stroke:#cb4b16,stroke-width:1px,color:#839496
```

## Flow Description

### Real-time Ingestion (Top to Bottom)
1. **Arr Stack**: Sonarr/Radarr send content to download clients
2. **Ingestion**: Download client drops file to watch directory
3. **JellyWatch Core**: 
   - File watcher detects new file
   - Name parser extracts title/year/episode info via regex
   - Confidence check: if < 0.8, queries local Ollama AI
   - Organizer queries Sonarr/Radarr for metadata verification
   - Renames file to Jellyfin standards, moves to appropriate drive
   - Indexes in SQLite database

### Library Maintenance (CLI Commands)
Run periodically on existing library:
- **scan**: Re-index all files across drives
- **audit**: Find low-confidence parses, suggest AI fixes
- **duplicates**: Find same content on multiple drives, keep best quality
- **consolidate**: Find scattered series (S01 on disk1, S02 on disk2), suggest merging

### Multi-Drive Topology
Library spans multiple drives. Database tracks location of every file. Maintenance operations read from DB, operate on filesystem, update DB with new locations.
