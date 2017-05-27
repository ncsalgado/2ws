# 2ws
2 way sync fs

File synchronization tool for Android, OSX, Unix, and Windows that allows a set of files and folders to be replicated by a server on several clients, modified separately, to be updated automatically or on demand, propagating the changes to each replica.

This type of synchronization requires a server that will contain the universe of the files to replicate.

Clients, depending on the user-defined configuration, may or may not have the same universe as the server.

Today several tools allow you to synchronize files, namely Dropbox, Google Drive, Unison, git, subversion, owncloud, rsync, etc. However none of them meets the set of characteristics proposed:
1. Run on various operating systems, including mobile systems.
2. Allow automatic or on-demand synchronization.
3. Allow synchronization between client and server and in the event of a collision that can not be resolved automatically, both changes are replicated, and the change left to the user later review.
4. Customers do not need to have the complete replicas depending on the type of client  and can only receive new files and / or only changed and / or deleted files. Clients can only send new and / or changed and / or deleted files to the server.
5. Be able to replicate the files to another folder on the client.
6. Can be installed on local servers.
7. GNU Free code.