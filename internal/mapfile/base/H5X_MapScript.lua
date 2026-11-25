-- h5-mapgen: the military posts take the faction of the town of the player whose area they stand in.
-- The game turns a random dwelling linked to a player, but not a military post: each post is placed as the
-- Haven one and converted here, like H5X converts dwellings.

MAPGEN_POSTS = {
{{POSTS}}}
MAPGEN_TOWN_TYPES = {"TOWN_HEAVEN", "TOWN_PRESERVE", "TOWN_INFERNO", "TOWN_NECROMANCY", "TOWN_ACADEMY", "TOWN_DUNGEON", "TOWN_FORTRESS", "TOWN_STRONGHOLD"}
MAPGEN_TOWN_IDS = {TOWN_HEAVEN, TOWN_PRESERVE, TOWN_INFERNO, TOWN_NECROMANCY, TOWN_ACADEMY, TOWN_DUNGEON, TOWN_FORTRESS, TOWN_STRONGHOLD}

-- MapgenPlayerFaction returns the faction (1 to 8, H5X order) of a player's town, or nil.
function MapgenPlayerFaction(player)
	for f = 1, 8 do
		local names = GetObjectNamesByType(MAPGEN_TOWN_TYPES[f])
		for _, town in names do
			if GetObjectOwner(town) == player then
				return f
			end
		end
	end
	return nil
end

function MapgenConvertPosts()
	sleep(1)
	for _, post in MAPGEN_POSTS do
		local f = MapgenPlayerFaction(post[2])
		if f and IsObjectExists(post[1]) then
			ReplaceDwelling(post[1], MAPGEN_TOWN_IDS[f])
			-- H5X's town conversion keeps the faction of every convertible dwelling
			if MAP_CONVERTIBLES and MAP_CONVERTIBLES[post[1]] then
				MAP_CONVERTIBLES[post[1]].faction = f
			end
		end
	end
end

startThread(MapgenConvertPosts)
