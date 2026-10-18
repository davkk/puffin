pragma foreign_keys = on;

create table if not exists mailbox (
    id integer primary key,
    name text not null,
    uid_validity integer default 0,
    uid_next integer default 1,
    highest_modseq integer default 0,
    last_sync datetime,
    unique (name)
);

create table if not exists message (
    id integer primary key,
    mailbox_id integer not null,
    uid integer not null,
    path text,
    flags text default '',
    modseq integer default 0,
    subject text,
    from_addr text,
    date datetime,
    body text,
    constraint fk_mailbox_id foreign key (mailbox_id) references mailbox (id) on delete cascade,
    unique (mailbox_id, uid)
);
