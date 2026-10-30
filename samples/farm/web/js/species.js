// The farm's species, housing and land, as plain data. Herds are counts, not individuals, so a farm
// can hold millions; the scene draws a few animals to stand for each herd. Imports nothing and
// touches no globals on load.
//
// A species: female/male/young are the display names; product is what each adult female makes,
// worth value coins every seconds; birth is the young per breeding female per second; a herd
// breeds min(females, males * matesPerMale) females; grow is the seconds a young one takes to grow
// up; life is the mean seconds an adult lives; price is the market price of one female or male;
// parcel is the land parcel (index into LAND) where the species lives.
//
// A housing tier: capacity is the most animals (adults, young and leased) the herd may have;
// cost is the price of moving up to it (the first tier is free); look says how housing.js draws
// it: shape is one of 'none', 'shed', 'barn', 'block', 'tower', 'burj', 'sail', 'mountain'; w and
// h are its size in units; wall and roof are PALETTE keys.

export const SPECIES = {
  cow: {
    female: 'cow', male: 'bull', young: 'calf', product: 'milk',
    value: 3, seconds: 10, birth: 1 / 150, matesPerMale: 20, grow: 60, life: 1080,
    price: { female: 30, male: 60 }, parcel: 0,
    tiers: [
      { name: 'Pasture', capacity: 10, cost: 0, look: { shape: 'none', w: 72, h: 60, wall: '8', roof: '2' } },
      { name: 'Cow Stables', capacity: 60, cost: 400, look: { shape: 'shed', w: 72, h: 34, wall: '4', roof: '5' } },
      { name: 'Big Red Barn', capacity: 400, cost: 4000, look: { shape: 'barn', w: 72, h: 60, wall: '8', roof: '2' } },
      { name: 'Dairy Ranch', capacity: 3000, cost: 40000, look: { shape: 'barn', w: 76, h: 66, wall: '7', roof: '8' } },
      { name: 'Cow Hotel', capacity: 25000, cost: 2e6, look: { shape: 'block', w: 64, h: 72, wall: 'f', roof: '8' } },
      { name: 'Moo York Tower', capacity: 250000, cost: 8e7, look: { shape: 'tower', w: 44, h: 80, wall: '6', roof: '5' } },
      { name: 'Burj Moolifa', capacity: 2.5e6, cost: 3e9, look: { shape: 'burj', w: 48, h: 82, wall: '7', roof: '0' } },
    ],
  },
  chicken: {
    female: 'hen', male: 'cock', young: 'chick', product: 'eggs',
    value: 1, seconds: 12, birth: 1 / 80, matesPerMale: 12, grow: 40, life: 640,
    price: { female: 8, male: 20 }, parcel: 0,
    tiers: [
      { name: 'Coop', capacity: 12, cost: 0, look: { shape: 'none', w: 32, h: 26, wall: '4', roof: '8' } },
      { name: 'Hen House', capacity: 80, cost: 250, look: { shape: 'shed', w: 32, h: 30, wall: '9', roof: '4' } },
      { name: 'Chicken Barn', capacity: 600, cost: 3000, look: { shape: 'barn', w: 40, h: 44, wall: 'a', roof: '4' } },
      { name: 'Egg Factory', capacity: 5000, cost: 35000, look: { shape: 'block', w: 40, h: 52, wall: '6', roof: '5' } },
      { name: 'Hen Hotel', capacity: 40000, cost: 1.5e6, look: { shape: 'block', w: 36, h: 62, wall: 'e', roof: '8' } },
      { name: 'Eggpire State Building', capacity: 400000, cost: 6e7, look: { shape: 'tower', w: 30, h: 78, wall: '7', roof: '9' } },
      { name: 'Burj Al Hen', capacity: 4e6, cost: 2.5e9, look: { shape: 'sail', w: 32, h: 82, wall: '7', roof: '8' } },
    ],
  },
  sheep: {
    female: 'ewe', male: 'ram', young: 'lamb', product: 'wool',
    value: 12, seconds: 20, birth: 1 / 150, matesPerMale: 25, grow: 80, life: 1400,
    price: { female: 200, male: 350 }, parcel: 1,
    tiers: [
      { name: 'Sheep Pen', capacity: 10, cost: 0, look: { shape: 'shed', w: 56, h: 24, wall: '4', roof: '5' } },
      { name: 'Shearing Shed', capacity: 80, cost: 5000, look: { shape: 'barn', w: 60, h: 40, wall: '6', roof: '5' } },
      { name: 'Wool Mill', capacity: 800, cost: 60000, look: { shape: 'block', w: 56, h: 56, wall: '7', roof: '4' } },
      { name: 'Fleece Palace', capacity: 10000, cost: 5e6, look: { shape: 'tower', w: 56, h: 70, wall: '7', roof: 'c' } },
      { name: 'Baa-tican City', capacity: 150000, cost: 3e8, look: { shape: 'tower', w: 64, h: 82, wall: 'f', roof: 'a' } },
    ],
  },
  goat: {
    female: 'nanny', male: 'billy', young: 'kid', product: 'cheese',
    value: 40, seconds: 30, birth: 1 / 180, matesPerMale: 15, grow: 90, life: 1500,
    price: { female: 800, male: 1200 }, parcel: 2,
    tiers: [
      { name: 'Goat Shed', capacity: 10, cost: 0, look: { shape: 'shed', w: 56, h: 24, wall: '5', roof: '4' } },
      { name: 'Goat Barn', capacity: 80, cost: 50000, look: { shape: 'barn', w: 60, h: 40, wall: '4', roof: '5' } },
      { name: 'Cheese Cave', capacity: 800, cost: 600000, look: { shape: 'mountain', w: 70, h: 50, wall: '5', roof: 'a' } },
      { name: 'Goat Castle', capacity: 10000, cost: 4e7, look: { shape: 'tower', w: 60, h: 70, wall: '6', roof: '5' } },
      { name: 'Mount Goatmore', capacity: 150000, cost: 2e9, look: { shape: 'mountain', w: 80, h: 82, wall: '6', roof: '7' } },
    ],
  },
};

// KINDS lists the herd species in the order the shop and the HUD show them.
export const KINDS = ['cow', 'chicken', 'sheep', 'goat'];

// LAND is the farm's parcels, left to right, each PARCEL_W units wide. The first is owned from the
// start; buying the next one extends the farm to the right and brings its species.
export const LAND = [
  { name: 'Gossip Acres', cost: 0 },
  { name: 'Sheep Meadow', cost: 3000 },
  { name: 'Goat Hill', cost: 50000 },
];
export const PARCEL_W = 320;
